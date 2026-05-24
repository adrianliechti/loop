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
		"/v1.45/containers/create": true,
		"/containers/json":         false,
		"/images/create":           false,
	}

	for path, want := range tests {
		if got := isContainerCreate(path); got != want {
			t.Fatalf("isContainerCreate(%q) = %v, want %v", path, got, want)
		}
	}
}
