package dockerproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

type Resolver interface {
	Resolve(ctx context.Context, source string) (string, error)
}

type PortForwarder interface {
	ForwardPort(ctx context.Context, localAddr string, localPort, remotePort int) error
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

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && isContainerCreate(r.URL.Path) {
			if err := rewriteCreateRequest(r, resolver); err != nil {
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

func rewriteCreateRequest(r *http.Request, resolver Resolver) error {
	data, err := io.ReadAll(r.Body)

	if err != nil {
		return err
	}

	r.Body.Close()

	rewritten, err := RewriteCreatePayload(r.Context(), data, resolver)

	if err != nil {
		return err
	}

	r.Body = io.NopCloser(bytes.NewReader(rewritten))
	r.ContentLength = int64(len(rewritten))
	r.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))

	return nil
}

func RewriteCreatePayload(ctx context.Context, data []byte, resolver Resolver) ([]byte, error) {
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
		if err := forwardPorts(ctx, hostConfig, forwarder); err != nil {
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
	if !isAbsLocalPath(source) {
		var err error

		source, err = filepath.Abs(source)

		if err != nil {
			return "", err
		}
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

func forwardPorts(ctx context.Context, hostConfig map[string]any, forwarder PortForwarder) error {
	raw, ok := hostConfig["PortBindings"]

	if !ok || raw == nil {
		return nil
	}

	bindings, ok := raw.(map[string]any)

	if !ok {
		return nil
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

			if hostPort == "" {
				continue
			}

			localPort, ok := parsePort(hostPort)

			if !ok {
				continue
			}

			hostIP, _ := entry["HostIp"].(string)

			if err := forwarder.ForwardPort(ctx, hostIP, localPort, localPort); err != nil {
				return err
			}
		}
	}

	return nil
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
