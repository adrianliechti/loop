package dockerproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
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
	})

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

func TestRewriteCreatePayloadRewritesRelativeBindSources(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	data := []byte(`{
		"Image": "alpine",
		"HostConfig": {
			"Binds": [
				"./project:/work:ro",
				"cache:/cache"
			],
			"Mounts": [
				{"Type": "bind", "Source": "mountdir", "Target": "/mountdir"}
			]
		}
	}`)

	rewritten, err := RewriteCreatePayload(context.Background(), data, fakeResolver{
		filepath.Join(dir, "project"):  "/data/project",
		filepath.Join(dir, "mountdir"): "/data/mountdir",
	})

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

	if got, want := payload.HostConfig.Binds[1], "cache:/cache"; got != want {
		t.Fatalf("bind[1] = %q, want %q", got, want)
	}

	if got, want := payload.HostConfig.Mounts[0].Source, "/data/mountdir"; got != want {
		t.Fatalf("mount[0].source = %q, want %q", got, want)
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
	})

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

	rewritten, err := RewriteCreatePayload(context.Background(), data, fakeResolver{})

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
				"53/udp": [{"HostPort": "10053"}],
				"443/tcp": [{"HostPort": ""}]
			}
		}
	}`)

	resolver := &fakeResolverWithPorts{}

	if _, err := RewriteCreatePayload(context.Background(), data, resolver); err != nil {
		t.Fatal(err)
	}

	if len(resolver.forwards) != 1 {
		t.Fatalf("forwards = %d, want 1", len(resolver.forwards))
	}

	if got, want := resolver.forwards[0], "127.0.0.1:18080:18080"; got != want {
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
