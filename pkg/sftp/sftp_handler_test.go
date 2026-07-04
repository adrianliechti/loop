package sftp

import (
	"os"
	"path/filepath"
	"testing"
)

func openRoot(t *testing.T) *os.Root {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { root.Close() })

	return root
}

func TestResolvePicksLongestMountTarget(t *testing.T) {
	base := openRoot(t)
	app := openRoot(t)
	deps := openRoot(t)

	// Registration order must not matter: /app/node_modules is nested inside
	// /app but routes to its own source.
	h := &handler{
		root: base,
		mounts: []rootMount{
			{root: app, target: "app"},
			{root: deps, target: "app/node_modules"},
		},
	}

	tests := []struct {
		path string
		root *os.Root
		name string
	}{
		{path: "/app", root: app, name: "."},
		{path: "/app/main.go", root: app, name: "main.go"},
		{path: "/app/node_modules", root: deps, name: "."},
		{path: "/app/node_modules/lib/index.js", root: deps, name: "lib/index.js"},
		{path: "/appendix", root: base, name: "appendix"},
		{path: "/other", root: base, name: "other"},
	}

	for _, test := range tests {
		root, name, err := h.resolve(test.path)

		if err != nil {
			t.Fatalf("resolve(%q): %v", test.path, err)
		}

		if root != test.root || name != test.name {
			t.Fatalf("resolve(%q) routed to wrong mount (name %q, want %q)", test.path, name, test.name)
		}
	}
}

func TestResolveWithoutBaseRoot(t *testing.T) {
	app := openRoot(t)

	h := &handler{
		mounts: []rootMount{
			{root: app, target: "app"},
		},
	}

	if _, _, err := h.resolve("/app/main.go"); err != nil {
		t.Fatalf("mount path: %v", err)
	}

	if _, _, err := h.resolve("/outside"); err == nil {
		t.Fatal("expected error for path outside mounts on mounts-only server")
	}
}

func TestResolveNestedMountServesOwnSource(t *testing.T) {
	appDir := t.TempDir()
	depsDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(appDir, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(appDir, "node_modules", "from-app"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(depsDir, "from-deps"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	appRoot, err := os.OpenRoot(appDir)

	if err != nil {
		t.Fatal(err)
	}

	defer appRoot.Close()

	depsRoot, err := os.OpenRoot(depsDir)

	if err != nil {
		t.Fatal(err)
	}

	defer depsRoot.Close()

	h := &handler{
		root: appRoot,
		mounts: []rootMount{
			{root: appRoot, target: "app"},
			{root: depsRoot, target: "app/node_modules"},
		},
	}

	root, name, err := h.resolve("/app/node_modules/from-deps")

	if err != nil {
		t.Fatal(err)
	}

	if _, err := root.Stat(name); err != nil {
		t.Fatalf("nested mount served from wrong source: %v", err)
	}
}
