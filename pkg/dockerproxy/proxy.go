package dockerproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/adrianliechti/loop/pkg/system"
)

type Resolver interface {
	Resolve(ctx context.Context, source string) (string, error)
}

type PortForwarder interface {
	ForwardPort(ctx context.Context, localAddr string, localPort, remotePort int) error
}

// ImageInspector reports the ports an image EXPOSEs. `docker run -P` sends no
// ExposedPorts in the create payload for image-declared ports, so the proxy
// has to ask the daemon to know what to publish.
type ImageInspector interface {
	ExposedPorts(ctx context.Context, image string) ([]string, error)
}

func Serve(ctx context.Context, addr, target string, resolver Resolver, ready chan<- struct{}) error {
	targetURL, err := url.Parse(target)

	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", addr)

	if err != nil {
		return err
	}

	if ready != nil {
		close(ready)
	}

	images := &daemonImages{
		base:   strings.TrimSuffix(target, "/"),
		client: &http.Client{Timeout: 10 * time.Second},
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && isContainerCreate(r.URL.Path) {
			if err := rewriteCreateRequest(r, resolver, images); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		proxy.ServeHTTP(w, r)
	})

	server := &http.Server{Handler: handler}

	go func() {
		<-ctx.Done()
		server.Close()
	}()

	err = server.Serve(listener)

	if err == nil || errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}

	return err
}

func isContainerCreate(p string) bool {
	p = path.Clean("/" + strings.TrimPrefix(p, "/"))

	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")

	if len(parts) == 2 {
		return parts[0] == "containers" && parts[1] == "create"
	}

	if len(parts) == 3 {
		return isAPIVersion(parts[0]) && parts[1] == "containers" && parts[2] == "create"
	}

	return false
}

func isAPIVersion(part string) bool {
	if len(part) < 2 || part[0] != 'v' {
		return false
	}

	for _, r := range part[1:] {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}

	return true
}

func rewriteCreateRequest(r *http.Request, resolver Resolver, images ImageInspector) error {
	data, err := io.ReadAll(r.Body)

	if err != nil {
		return err
	}

	r.Body.Close()

	rewritten, err := RewriteCreatePayload(r.Context(), data, resolver, images)

	if err != nil {
		return err
	}

	r.Body = io.NopCloser(bytes.NewReader(rewritten))
	r.ContentLength = int64(len(rewritten))
	r.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))

	return nil
}

func RewriteCreatePayload(ctx context.Context, data []byte, resolver Resolver, images ImageInspector) ([]byte, error) {
	var payload map[string]any

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}

	hostConfig, _ := payload["HostConfig"].(map[string]any)

	if hostConfig == nil {
		return data, nil
	}

	if err := rewriteBinds(ctx, hostConfig, resolver); err != nil {
		return nil, err
	}

	if err := rewriteMounts(ctx, hostConfig, resolver); err != nil {
		return nil, err
	}

	if forwarder, ok := resolver.(PortForwarder); ok {
		if err := forwardPorts(ctx, payload, hostConfig, forwarder, images); err != nil {
			return nil, err
		}
	}

	return json.Marshal(payload)
}

func rewriteBinds(ctx context.Context, hostConfig map[string]any, resolver Resolver) error {
	raw, ok := hostConfig["Binds"]

	if !ok || raw == nil {
		return nil
	}

	binds, ok := raw.([]any)

	if !ok {
		return nil
	}

	for i, rawBind := range binds {
		bind, ok := rawBind.(string)

		if !ok {
			continue
		}

		source, rest, ok := splitBind(bind)

		if !ok || !isLocalVolumeSource(source) {
			continue
		}

		remote, err := resolveSource(ctx, resolver, source)

		if err != nil {
			return err
		}

		binds[i] = remote + rest
	}

	hostConfig["Binds"] = binds
	return nil
}

func rewriteMounts(ctx context.Context, hostConfig map[string]any, resolver Resolver) error {
	raw, ok := hostConfig["Mounts"]

	if !ok || raw == nil {
		return nil
	}

	mounts, ok := raw.([]any)

	if !ok {
		return nil
	}

	for _, rawMount := range mounts {
		mount, ok := rawMount.(map[string]any)

		if !ok {
			continue
		}

		typ, _ := mount["Type"].(string)

		if typ != "bind" {
			continue
		}

		source, _ := mount["Source"].(string)

		if source == "" {
			continue
		}

		remote, err := resolveSource(ctx, resolver, source)

		if err != nil {
			return err
		}

		mount["Source"] = remote
	}

	hostConfig["Mounts"] = mounts
	return nil
}

func splitBind(bind string) (string, string, bool) {
	if bind == "" {
		return "", "", false
	}

	start := 0

	if hasWindowsDrivePrefix(bind) {
		start = 2
	}

	idx := strings.Index(bind[start:], ":")

	if idx < 0 {
		return "", "", false
	}

	idx += start

	if idx <= 0 {
		return "", "", false
	}

	return bind[:idx], bind[idx:], true
}

func isLocalVolumeSource(source string) bool {
	return isAbsLocalPath(source) || strings.HasPrefix(source, ".")
}

func resolveSource(ctx context.Context, resolver Resolver, source string) (string, error) {
	// The Docker CLI resolves relative paths before sending them and the
	// daemon rejects them. Resolving here would use the proxy's working
	// directory — not the client's — and silently mount the wrong directory,
	// so mirror the daemon and reject instead.
	if !isAbsLocalPath(source) {
		return "", fmt.Errorf("relative bind source %q is not supported: use an absolute path", source)
	}

	return resolver.Resolve(ctx, source)
}

func isAbsLocalPath(source string) bool {
	return filepath.IsAbs(source) || strings.HasPrefix(source, "/") || hasWindowsDrivePrefix(source) || isWindowsUNCPath(source)
}

func hasWindowsDrivePrefix(source string) bool {
	return len(source) >= 3 && isASCIILetter(source[0]) && source[1] == ':' && isPathSeparator(source[2])
}

func isWindowsUNCPath(source string) bool {
	return len(source) >= 2 && isPathSeparator(source[0]) && isPathSeparator(source[1])
}

func isASCIILetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func isPathSeparator(b byte) bool {
	return b == '/' || b == '\\'
}

func forwardPorts(ctx context.Context, payload, hostConfig map[string]any, forwarder PortForwarder, images ImageInspector) error {
	bindings, _ := hostConfig["PortBindings"].(map[string]any)

	// `docker run -P` publishes every exposed port on a daemon-assigned random
	// port, which would only exist inside the remote pod. Synthesize explicit
	// bindings instead so the ports get concrete, forwardable host ports
	// below. Ports declared only via the image's EXPOSE are not in the create
	// payload, so those are fetched from the daemon's image config. Only
	// forwardable (TCP) ports are synthesized: publishing a UDP port the
	// SSH tunnel cannot carry would just report an unreachable binding.
	if publishAll, _ := hostConfig["PublishAllPorts"].(bool); publishAll {
		if bindings == nil {
			bindings = map[string]any{}
		}

		exposed := map[string]struct{}{}

		if payloadPorts, ok := payload["ExposedPorts"].(map[string]any); ok {
			for port := range payloadPorts {
				exposed[port] = struct{}{}
			}
		}

		if images != nil {
			if image, _ := payload["Image"].(string); image != "" {
				imagePorts, err := images.ExposedPorts(ctx, image)

				if err != nil {
					return fmt.Errorf("cannot determine exposed ports of %q for --publish-all: %w", image, err)
				}

				for _, port := range imagePorts {
					exposed[port] = struct{}{}
				}
			}
		}

		for port := range exposed {
			if _, ok := parseContainerPort(port); !ok {
				continue
			}

			if _, ok := bindings[port]; !ok {
				bindings[port] = []any{map[string]any{"HostIp": "", "HostPort": ""}}
			}
		}

		hostConfig["PublishAllPorts"] = false
		hostConfig["PortBindings"] = bindings
	}

	for port, rawEntries := range bindings {
		if _, ok := parseContainerPort(port); !ok {
			continue
		}

		entries, ok := rawEntries.([]any)

		if !ok {
			continue
		}

		for _, rawEntry := range entries {
			entry, ok := rawEntry.(map[string]any)

			if !ok {
				continue
			}

			hostPort, _ := entry["HostPort"].(string)

			// The daemon assigns empty host ports (and picks within ranges) at
			// start time inside the remote pod, where the result is
			// unreachable. Pick a concrete free local port up front instead so
			// the same port exists on both ends, and record it in the payload
			// so `docker port` reports reality.
			if concrete, err := concreteHostPort(hostPort); err != nil {
				return err
			} else if concrete != hostPort {
				hostPort = concrete
				entry["HostPort"] = hostPort
			}

			localPort, ok := parsePort(hostPort)

			if !ok {
				continue
			}

			hostIP, _ := entry["HostIp"].(string)

			// Unset means "all interfaces" to the daemon, but the local
			// forward defaults to loopback: write the address actually bound
			// so `docker ps` doesn't advertise an unreachable wildcard.
			// Explicit wildcard/IPv6 requests pass through unchanged.
			if hostIP == "" {
				hostIP = "127.0.0.1"
				entry["HostIp"] = hostIP
			}

			if err := forwarder.ForwardPort(ctx, hostIP, localPort, localPort); err != nil {
				return err
			}
		}
	}

	return nil
}

// concreteHostPort turns "" and "lo-hi" range specs into a specific free
// local port; concrete values are returned unchanged.
func concreteHostPort(hostPort string) (string, error) {
	if hostPort == "" {
		port, err := system.FreePort(0)

		if err != nil {
			return "", err
		}

		return strconv.Itoa(port), nil
	}

	lo, hi, ok := strings.Cut(hostPort, "-")

	if !ok {
		return hostPort, nil
	}

	loPort, loOK := parsePort(lo)
	hiPort, hiOK := parsePort(hi)

	if !loOK || !hiOK || hiPort < loPort {
		return hostPort, nil
	}

	for p := loPort; p <= hiPort; p++ {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))

		if err != nil {
			continue
		}

		listener.Close()

		return strconv.Itoa(p), nil
	}

	return "", fmt.Errorf("no free local port in range %s", hostPort)
}

// daemonImages resolves image EXPOSE declarations via the daemon's image
// inspect endpoint.
type daemonImages struct {
	base   string
	client *http.Client
}

func (d *daemonImages) ExposedPorts(ctx context.Context, image string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.base+"/images/"+url.PathEscape(image)+"/json", nil)

	if err != nil {
		return nil, err
	}

	resp, err := d.client.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image inspect %q: %s", image, resp.Status)
	}

	var payload struct {
		Config struct {
			ExposedPorts map[string]struct{} `json:"ExposedPorts"`
		} `json:"Config"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	ports := make([]string, 0, len(payload.Config.ExposedPorts))

	for port := range payload.Config.ExposedPorts {
		ports = append(ports, port)
	}

	return ports, nil
}

func parseContainerPort(port string) (int, bool) {
	port = strings.TrimSuffix(port, "/tcp")

	if strings.Contains(port, "/") {
		return 0, false
	}

	return parsePort(port)
}

func parsePort(port string) (int, bool) {
	value, err := strconv.Atoi(port)

	if err != nil {
		return 0, false
	}

	return value, value > 0 && value <= 65535
}
