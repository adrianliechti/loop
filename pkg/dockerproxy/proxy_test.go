package dockerproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

type fakeResolver map[string]string

func (r fakeResolver) Resolve(ctx context.Context, source string) (string, error) {
	return r[source], nil
}

type fakeResolverWithPorts struct {
	fakeResolver
	forwards []string
}

func (r *fakeResolverWithPorts) ForwardPort(ctx context.Context, localAddr string, localPort, remotePort int) error {
	r.forwards = append(r.forwards, fmt.Sprintf("%s:%d:%d", localAddr, localPort, remotePort))
	return nil
}

func TestRewriteCreatePayloadRewritesBindSources(t *testing.T) {
	data := []byte(`{
		"Image": "alpine",
		"HostConfig": {
			"Binds": [
				"/Users/me/project:/work:ro",
				"cache:/cache"
			],
			"Mounts": [
				{"Type": "bind", "Source": "/Users/me/file.txt", "Target": "/file"},
				{"Type": "volume", "Source": "named", "Target": "/named"}
			]
		}
	}`)

	rewritten, err := RewriteCreatePayload(context.Background(), data, fakeResolver{
		"/Users/me/project":  "/data/project",
		"/Users/me/file.txt": "/data/file.txt",
	}, nil)

	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		HostConfig struct {
			Binds  []string `json:"Binds"`
			Mounts []struct {
				Type   string `json:"Type"`
				Source string `json:"Source"`
				Target string `json:"Target"`
			} `json:"Mounts"`
		} `json:"HostConfig"`
	}

	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatal(err)
	}

	if got, want := payload.HostConfig.Binds[0], "/data/project:/work:ro"; got != want {
		t.Fatalf("bind[0] = %q, want %q", got, want)
	}

	if got, want := payload.HostConfig.Binds[1], "cache:/cache"; got != want {
		t.Fatalf("bind[1] = %q, want %q", got, want)
	}

	if got, want := payload.HostConfig.Mounts[0].Source, "/data/file.txt"; got != want {
		t.Fatalf("mount[0].source = %q, want %q", got, want)
	}

	if got, want := payload.HostConfig.Mounts[1].Source, "named"; got != want {
		t.Fatalf("mount[1].source = %q, want %q", got, want)
	}
}

func TestRewriteCreatePayloadRejectsRelativeBindSources(t *testing.T) {
	// The proxy cannot know the Docker client's working directory, so relative
	// bind sources must be rejected (as the daemon does) instead of resolved
	// against the proxy's own cwd.
	data := []byte(`{
		"Image": "alpine",
		"HostConfig": {
			"Binds": [
				"./project:/work:ro"
			]
		}
	}`)

	if _, err := RewriteCreatePayload(context.Background(), data, fakeResolver{}, nil); err == nil {
		t.Fatal("expected error for relative bind source")
	}

	data = []byte(`{
		"Image": "alpine",
		"HostConfig": {
			"Mounts": [
				{"Type": "bind", "Source": "mountdir", "Target": "/mountdir"}
			]
		}
	}`)

	if _, err := RewriteCreatePayload(context.Background(), data, fakeResolver{}, nil); err == nil {
		t.Fatal("expected error for relative mount source")
	}
}

func TestRewriteCreatePayloadRewritesWindowsBindSources(t *testing.T) {
	data := []byte(`{
		"Image": "alpine",
		"HostConfig": {
			"Binds": [
				"C:\\Users\\me\\project:/work:ro",
				"D:/data:/data"
			],
			"Mounts": [
				{"Type": "bind", "Source": "C:\\Users\\me\\file.txt", "Target": "/file"}
			]
		}
	}`)

	rewritten, err := RewriteCreatePayload(context.Background(), data, fakeResolver{
		`C:\Users\me\project`:  "/data/project",
		"D:/data":              "/data/d-drive",
		`C:\Users\me\file.txt`: "/data/file.txt",
	}, nil)

	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		HostConfig struct {
			Binds  []string `json:"Binds"`
			Mounts []struct {
				Source string `json:"Source"`
			} `json:"Mounts"`
		} `json:"HostConfig"`
	}

	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatal(err)
	}

	if got, want := payload.HostConfig.Binds[0], "/data/project:/work:ro"; got != want {
		t.Fatalf("bind[0] = %q, want %q", got, want)
	}

	if got, want := payload.HostConfig.Binds[1], "/data/d-drive:/data"; got != want {
		t.Fatalf("bind[1] = %q, want %q", got, want)
	}

	if got, want := payload.HostConfig.Mounts[0].Source, "/data/file.txt"; got != want {
		t.Fatalf("mount[0].source = %q, want %q", got, want)
	}
}

func TestSplitBind(t *testing.T) {
	tests := map[string]struct {
		source string
		rest   string
		ok     bool
	}{
		"/Users/me/project:/work:ro":      {source: "/Users/me/project", rest: ":/work:ro", ok: true},
		"./project:/work":                 {source: "./project", rest: ":/work", ok: true},
		"cache:/cache":                    {source: "cache", rest: ":/cache", ok: true},
		`C:\Users\me\project:/work:ro`:    {source: `C:\Users\me\project`, rest: ":/work:ro", ok: true},
		"C:/Users/me/project:/work:ro":    {source: "C:/Users/me/project", rest: ":/work:ro", ok: true},
		`\\server\share\project:/work:ro`: {source: `\\server\share\project`, rest: ":/work:ro", ok: true},
		"":                                {ok: false},
		"missing-target":                  {ok: false},
	}

	for bind, want := range tests {
		source, rest, ok := splitBind(bind)

		if ok != want.ok {
			t.Fatalf("splitBind(%q) ok = %v, want %v", bind, ok, want.ok)
		}

		if source != want.source || rest != want.rest {
			t.Fatalf("splitBind(%q) = (%q, %q), want (%q, %q)", bind, source, rest, want.source, want.rest)
		}
	}
}

func TestRewriteCreatePayloadWithoutHostConfigPassesThrough(t *testing.T) {
	data := []byte(`{"Image":"alpine"}`)

	rewritten, err := RewriteCreatePayload(context.Background(), data, fakeResolver{}, nil)

	if err != nil {
		t.Fatal(err)
	}

	if string(rewritten) != string(data) {
		t.Fatalf("payload changed: %s", rewritten)
	}
}

func TestRewriteCreatePayloadForwardsExplicitPortBindings(t *testing.T) {
	data := []byte(`{
		"Image": "nginx",
		"HostConfig": {
			"PortBindings": {
				"80/tcp": [{"HostIp": "127.0.0.1", "HostPort": "18080"}],
				"53/udp": [{"HostPort": "10053"}]
			}
		}
	}`)

	resolver := &fakeResolverWithPorts{}

	if _, err := RewriteCreatePayload(context.Background(), data, resolver, nil); err != nil {
		t.Fatal(err)
	}

	if len(resolver.forwards) != 1 {
		t.Fatalf("forwards = %d, want 1", len(resolver.forwards))
	}

	if got, want := resolver.forwards[0], "127.0.0.1:18080:18080"; got != want {
		t.Fatalf("forward = %q, want %q", got, want)
	}
}

func TestRewriteCreatePayloadAssignsEmptyHostPorts(t *testing.T) {
	// An empty host port means "let the daemon pick one" — inside the remote
	// pod, where it would be unreachable. The proxy must pick a concrete local
	// port instead, rewrite the payload, and forward it.
	data := []byte(`{
		"Image": "nginx",
		"HostConfig": {
			"PortBindings": {
				"443/tcp": [{"HostPort": ""}]
			}
		}
	}`)

	resolver := &fakeResolverWithPorts{}

	rewritten, err := RewriteCreatePayload(context.Background(), data, resolver, nil)

	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		HostConfig struct {
			PortBindings map[string][]struct {
				HostIP   string `json:"HostIp"`
				HostPort string `json:"HostPort"`
			} `json:"PortBindings"`
		} `json:"HostConfig"`
	}

	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatal(err)
	}

	binding := payload.HostConfig.PortBindings["443/tcp"][0]

	if binding.HostPort == "" {
		t.Fatal("host port was not assigned")
	}

	// The payload must reflect the loopback address the forward actually binds.
	if binding.HostIP != "127.0.0.1" {
		t.Fatalf("host ip = %q, want 127.0.0.1", binding.HostIP)
	}

	if len(resolver.forwards) != 1 {
		t.Fatalf("forwards = %d, want 1", len(resolver.forwards))
	}

	if got, want := resolver.forwards[0], fmt.Sprintf("127.0.0.1:%s:%s", binding.HostPort, binding.HostPort); got != want {
		t.Fatalf("forward = %q, want %q", got, want)
	}
}

func TestRewriteCreatePayloadPublishesAllExposedPorts(t *testing.T) {
	// docker run -P: bindings are synthesized from the payload's ExposedPorts,
	// PublishAllPorts is cleared so the daemon binds exactly those ports.
	data := []byte(`{
		"Image": "nginx",
		"ExposedPorts": {"80/tcp": {}},
		"HostConfig": {
			"PublishAllPorts": true
		}
	}`)

	resolver := &fakeResolverWithPorts{}

	rewritten, err := RewriteCreatePayload(context.Background(), data, resolver, nil)

	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		HostConfig struct {
			PublishAllPorts bool `json:"PublishAllPorts"`
			PortBindings    map[string][]struct {
				HostPort string `json:"HostPort"`
			} `json:"PortBindings"`
		} `json:"HostConfig"`
	}

	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatal(err)
	}

	if payload.HostConfig.PublishAllPorts {
		t.Fatal("PublishAllPorts was not cleared")
	}

	hostPort := payload.HostConfig.PortBindings["80/tcp"][0].HostPort

	if hostPort == "" {
		t.Fatal("host port was not assigned")
	}

	if len(resolver.forwards) != 1 {
		t.Fatalf("forwards = %d, want 1", len(resolver.forwards))
	}

	if got, want := resolver.forwards[0], fmt.Sprintf("127.0.0.1:%s:%s", hostPort, hostPort); got != want {
		t.Fatalf("forward = %q, want %q", got, want)
	}
}

type fakeImages map[string][]string

func (f fakeImages) ExposedPorts(ctx context.Context, image string) ([]string, error) {
	ports, ok := f[image]

	if !ok {
		return nil, fmt.Errorf("no such image %q", image)
	}

	return ports, nil
}

func TestRewriteCreatePayloadPublishesImageExposedPorts(t *testing.T) {
	// docker run -P sends no ExposedPorts for ports declared only via the
	// image's EXPOSE; those must be fetched from the daemon's image config.
	// Non-TCP ports cannot be carried by the tunnel and must not be published.
	data := []byte(`{
		"Image": "nginx",
		"HostConfig": {
			"PublishAllPorts": true
		}
	}`)

	resolver := &fakeResolverWithPorts{}
	images := fakeImages{"nginx": {"80/tcp", "53/udp"}}

	rewritten, err := RewriteCreatePayload(context.Background(), data, resolver, images)

	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		HostConfig struct {
			PublishAllPorts bool `json:"PublishAllPorts"`
			PortBindings    map[string][]struct {
				HostPort string `json:"HostPort"`
			} `json:"PortBindings"`
		} `json:"HostConfig"`
	}

	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatal(err)
	}

	if payload.HostConfig.PublishAllPorts {
		t.Fatal("PublishAllPorts was not cleared")
	}

	if _, ok := payload.HostConfig.PortBindings["53/udp"]; ok {
		t.Fatal("udp port was published but cannot be forwarded")
	}

	if payload.HostConfig.PortBindings["80/tcp"][0].HostPort == "" {
		t.Fatal("host port was not assigned for image-exposed port")
	}

	if len(resolver.forwards) != 1 {
		t.Fatalf("forwards = %d, want 1", len(resolver.forwards))
	}
}

func TestRewriteCreatePayloadResolvesHostPortRanges(t *testing.T) {
	// The daemon would pick a port within the range remotely, where it is
	// unreachable; a concrete free local port from the range must be chosen
	// and written back instead.
	data := []byte(`{
		"Image": "nginx",
		"HostConfig": {
			"PortBindings": {
				"80/tcp": [{"HostPort": "18100-18110"}]
			}
		}
	}`)

	resolver := &fakeResolverWithPorts{}

	rewritten, err := RewriteCreatePayload(context.Background(), data, resolver, nil)

	if err != nil {
		t.Fatal(err)
	}

	var payload struct {
		HostConfig struct {
			PortBindings map[string][]struct {
				HostPort string `json:"HostPort"`
			} `json:"PortBindings"`
		} `json:"HostConfig"`
	}

	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatal(err)
	}

	hostPort := payload.HostConfig.PortBindings["80/tcp"][0].HostPort
	port, ok := parsePort(hostPort)

	if !ok || port < 18100 || port > 18110 {
		t.Fatalf("host port %q not resolved within range", hostPort)
	}

	if len(resolver.forwards) != 1 {
		t.Fatalf("forwards = %d, want 1", len(resolver.forwards))
	}

	if got, want := resolver.forwards[0], fmt.Sprintf("127.0.0.1:%d:%d", port, port); got != want {
		t.Fatalf("forward = %q, want %q", got, want)
	}
}

func TestRewriteCreatePayloadKeepsExplicitWildcardHostIP(t *testing.T) {
	data := []byte(`{
		"Image": "nginx",
		"HostConfig": {
			"PortBindings": {
				"80/tcp": [{"HostIp": "0.0.0.0", "HostPort": "18080"}]
			}
		}
	}`)

	resolver := &fakeResolverWithPorts{}

	if _, err := RewriteCreatePayload(context.Background(), data, resolver, nil); err != nil {
		t.Fatal(err)
	}

	if len(resolver.forwards) != 1 {
		t.Fatalf("forwards = %d, want 1", len(resolver.forwards))
	}

	if got, want := resolver.forwards[0], "0.0.0.0:18080:18080"; got != want {
		t.Fatalf("forward = %q, want %q", got, want)
	}
}

func TestIsContainerCreate(t *testing.T) {
	tests := map[string]bool{
		"/containers/create":       true,
		"/containers/create/":      true,
		"/v1.45/containers/create": true,
		"/v1x/containers/create":   false,
		"/foo/containers/create":   false,
		"/containers/json":         false,
		"/images/create":           false,
	}

	for path, want := range tests {
		if got := isContainerCreate(path); got != want {
			t.Fatalf("isContainerCreate(%q) = %v, want %v", path, got, want)
		}
	}
}
