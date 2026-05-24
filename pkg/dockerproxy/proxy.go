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
		if isContainerCreate(r.URL.Path) {
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

	return p == "/containers/create" || strings.HasSuffix(p, "/containers/create")
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
	r.Header.Set("Content-Length", fmt.Sprintf("%d", len(rewritten)))

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

		if !ok || !isLocalBindSource(source) {
			continue
		}

		remote, err := resolver.Resolve(ctx, source)

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

		if !isLocalBindSource(source) {
			continue
		}

		remote, err := resolver.Resolve(ctx, source)

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

	idx := strings.Index(bind, ":")

	if idx <= 0 {
		return "", "", false
	}

	return bind[:idx], bind[idx:], true
}

func isLocalBindSource(source string) bool {
	return strings.HasPrefix(source, "/")
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
	if port == "" {
		return 0, false
	}

	var value int

	for _, r := range port {
		if r < '0' || r > '9' {
			return 0, false
		}

		value = value*10 + int(r-'0')
	}

	return value, value > 0 && value <= 65535
}
