//go:build integration

// Integration tests for the remote Docker stack.
//
// These tests stand up a real in-cluster daemon and drive it through the
// `loop` proxy via the local `docker` CLI. They exercise the bind/mount
// rewriting and remote-mount manager against scenarios that have caused
// confusion in the past (same source mapped to multiple targets, different
// sources colliding on the same target across containers, single-file
// binds, sibling directories, write-back from container to host, and the
// HostConfig.Mounts entry-point as opposed to HostConfig.Binds).
//
// Requirements:
//   - $KUBECONFIG (or ~/.kube/config) pointing at a writable cluster
//   - the `docker` CLI on $PATH
//   - egress to pull the dind / loop-tunnel / alpine / nginx images
//
// Invoke with:
//
//	go test -tags integration -v -timeout 20m ./pkg/remote/docker/
package docker

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adrianliechti/loop/pkg/dockerproxy"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/remotemount"
	"github.com/adrianliechti/loop/pkg/ssh"
	"github.com/google/uuid"

	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	testAlpineImage = "public.ecr.aws/docker/library/alpine:3.20"
	testNginxImage  = "public.ecr.aws/nginx/nginx:stable-alpine"
)

type integrationEnv struct {
	proxyAddr string

	client    kubernetes.Client
	namespace string
	name      string
}

func setupIntegrationEnv(t *testing.T) *integrationEnv {
	t.Helper()

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker CLI not available: %v", err)
	}

	client, err := kubernetes.NewFromFile("", "")

	if err != nil {
		t.Skipf("no kubeconfig available: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	name := "test-" + uuid.NewString()[:6]
	namespace := client.Namespace()

	t.Logf("creating daemon %s/%s", namespace, name)

	createCtx, createCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer createCancel()

	if err := Create(createCtx, client, &CreateOptions{
		Name:      name,
		Namespace: namespace,

		CPU:     resource.MustParse("500m"),
		Memory:  resource.MustParse("1Gi"),
		Storage: resource.MustParse("5Gi"),
	}); err != nil {
		t.Fatalf("create daemon: %v", err)
	}

	t.Cleanup(func() {
		delCtx, delCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer delCancel()

		if err := Delete(delCtx, client, namespace, name); err != nil {
			t.Logf("delete daemon: %v", err)
		}
	})

	podName := resourceName(name) + "-0"

	if _, err := client.WaitForPod(ctx, namespace, podName); err != nil {
		t.Fatalf("wait pod: %v", err)
	}

	sshPort, err := pickPort(0)

	if err != nil {
		t.Fatalf("pick ssh port: %v", err)
	}

	sshReady := make(chan struct{})
	sshDone := make(chan error, 1)

	go func() {
		sshDone <- client.PodPortForward(ctx, namespace, podName, "127.0.0.1", map[int]int{sshPort: 22}, sshReady)
	}()

	if err := waitForReady(ctx, sshReady, sshDone); err != nil {
		t.Fatalf("ssh port-forward: %v", err)
	}

	sshAddr := fmt.Sprintf("127.0.0.1:%d", sshPort)

	daemonPort, err := pickPort(0)

	if err != nil {
		t.Fatalf("pick daemon port: %v", err)
	}

	daemonReady := make(chan struct{})
	daemonDone := make(chan error, 1)

	go func() {
		c := ssh.New(sshAddr,
			ssh.WithLocalPortForward(ssh.PortForward{LocalPort: daemonPort, RemotePort: 2375}),
			ssh.WithReady(daemonReady),
		)
		daemonDone <- c.Run(ctx)
	}()

	if err := waitForReady(ctx, daemonReady, daemonDone); err != nil {
		t.Fatalf("daemon tunnel: %v", err)
	}

	if err := waitForDocker(ctx, daemonPort); err != nil {
		t.Fatalf("daemon ping: %v", err)
	}

	mounts := remotemount.NewManager(ctx, sshAddr, "/data")
	t.Cleanup(mounts.Close)

	proxyPort, err := pickPort(0)

	if err != nil {
		t.Fatalf("pick proxy port: %v", err)
	}

	proxyReady := make(chan struct{})
	proxyDone := make(chan error, 1)

	go func() {
		proxyDone <- dockerproxy.Serve(ctx, fmt.Sprintf("127.0.0.1:%d", proxyPort), fmt.Sprintf("http://127.0.0.1:%d", daemonPort), mounts, proxyReady)
	}()

	if err := waitForReady(ctx, proxyReady, proxyDone); err != nil {
		t.Fatalf("proxy: %v", err)
	}

	env := &integrationEnv{
		proxyAddr: fmt.Sprintf("tcp://127.0.0.1:%d", proxyPort),

		client:    client,
		namespace: namespace,
		name:      name,
	}

	t.Logf("daemon ready, pre-pulling %s", testAlpineImage)

	if stdout, stderr, err := env.dockerExec("pull", testAlpineImage); err != nil {
		t.Fatalf("warm pull %s: %v\nstdout: %s\nstderr: %s", testAlpineImage, err, stdout, stderr)
	}

	return env
}

func (env *integrationEnv) docker(t *testing.T, args ...string) string {
	t.Helper()

	stdout, stderr, err := env.dockerExec(args...)

	if err != nil {
		t.Fatalf("docker %s: %v\nstdout: %s\nstderr: %s", strings.Join(args, " "), err, stdout, stderr)
	}

	return strings.TrimRight(stdout, "\n")
}

func (env *integrationEnv) dockerExec(args ...string) (string, string, error) {
	cmd := exec.Command("docker", args...)

	// DOCKER_CONTEXT is cleared so the inherited host-side context (which
	// would normally win over DOCKER_HOST) does not redirect us to the
	// developer's regular daemon.
	cmd.Env = append(os.Environ(), "DOCKER_HOST="+env.proxyAddr, "DOCKER_CONTEXT=")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	return stdout.String(), stderr.String(), err
}

func (env *integrationEnv) removeContainer(name string) {
	env.dockerExec("rm", "-f", name)
}

func uniqueName(prefix string) string {
	return prefix + "-" + uuid.NewString()[:6]
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIntegration(t *testing.T) {
	env := setupIntegrationEnv(t)

	// One source dir, mounted at two different targets in the same container.
	// The mount manager should de-duplicate to a single sshfs and the proxy
	// should rewrite both Binds entries to the same remote path.
	t.Run("SameSource_TwoTargets_OneContainer", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "file.txt"), "shared\n")

		out := env.docker(t,
			"run", "--rm",
			"-v", dir+":/a:ro",
			"-v", dir+":/b:ro",
			testAlpineImage,
			"sh", "-c", "cat /a/file.txt; echo ---; cat /b/file.txt",
		)

		want := "shared\n---\nshared"

		if out != want {
			t.Fatalf("output = %q, want %q", out, want)
		}
	})

	// Two different source dirs collide on the same in-container target across
	// two separate containers. Each container must see only its own source.
	t.Run("DifferentSources_SameTarget_TwoContainers", func(t *testing.T) {
		dirA := t.TempDir()
		dirB := t.TempDir()
		writeFile(t, filepath.Join(dirA, "marker"), "A")
		writeFile(t, filepath.Join(dirB, "marker"), "B")

		nameA := uniqueName("loop-int-a")
		nameB := uniqueName("loop-int-b")
		t.Cleanup(func() { env.removeContainer(nameA) })
		t.Cleanup(func() { env.removeContainer(nameB) })

		env.docker(t, "run", "-d", "--name", nameA, "-v", dirA+":/shared:ro", testAlpineImage, "sleep", "120")
		env.docker(t, "run", "-d", "--name", nameB, "-v", dirB+":/shared:ro", testAlpineImage, "sleep", "120")

		if got := env.docker(t, "exec", nameA, "cat", "/shared/marker"); got != "A" {
			t.Fatalf("container A saw %q, want %q", got, "A")
		}

		if got := env.docker(t, "exec", nameB, "cat", "/shared/marker"); got != "B" {
			t.Fatalf("container B saw %q, want %q", got, "B")
		}
	})

	// Single regular file bind. The manager must mount the parent directory
	// and append the file suffix to the remote path.
	t.Run("SingleFileBind", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		writeFile(t, path, "key: value\n")

		out := env.docker(t,
			"run", "--rm",
			"-v", path+":/etc/config.yaml:ro",
			testAlpineImage,
			"cat", "/etc/config.yaml",
		)

		if out != "key: value" {
			t.Fatalf("output = %q", out)
		}
	})

	// A directory and a file *inside that same directory* are both bound.
	// The manager must dedupe: one sshfs serves both binds.
	t.Run("FileInsideAlreadyMountedDir", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "config"), "config-content\n")

		out := env.docker(t,
			"run", "--rm",
			"-v", dir+":/data:ro",
			"-v", filepath.Join(dir, "config")+":/etc/config:ro",
			testAlpineImage,
			"sh", "-c", "cat /data/config; echo ---; cat /etc/config",
		)

		want := "config-content\n---\nconfig-content"

		if out != want {
			t.Fatalf("output = %q, want %q", out, want)
		}
	})

	// Two sibling directories under a common parent — each mounted separately.
	// They share no mount and must remain isolated.
	t.Run("TwoSiblingDirectories", func(t *testing.T) {
		root := t.TempDir()
		a := filepath.Join(root, "a")
		b := filepath.Join(root, "b")

		for _, d := range []string{a, b} {
			if err := os.Mkdir(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}

		writeFile(t, filepath.Join(a, "tag"), "A\n")
		writeFile(t, filepath.Join(b, "tag"), "B\n")

		out := env.docker(t,
			"run", "--rm",
			"-v", a+":/ma:ro",
			"-v", b+":/mb:ro",
			testAlpineImage,
			"sh", "-c", "cat /ma/tag; echo ---; cat /mb/tag",
		)

		want := "A\n---\nB"

		if out != want {
			t.Fatalf("output = %q, want %q", out, want)
		}
	})

	// Container writes a file into a bind mount; the bytes must show up on
	// the developer's local filesystem (sshfs round-trips through SFTP).
	t.Run("WriteFromContainerToHost", func(t *testing.T) {
		dir := t.TempDir()

		env.docker(t,
			"run", "--rm",
			"-v", dir+":/out",
			testAlpineImage,
			"sh", "-c", "echo from-container > /out/output.txt",
		)

		b, err := os.ReadFile(filepath.Join(dir, "output.txt"))

		if err != nil {
			t.Fatal(err)
		}

		if got := strings.TrimSpace(string(b)); got != "from-container" {
			t.Fatalf("file content = %q", got)
		}
	})

	// Same as the basic bind test but uses --mount, which arrives in
	// HostConfig.Mounts (not Binds) and goes through a separate rewriter.
	t.Run("MountFlagBindType", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "x"), "via-mount")

		out := env.docker(t,
			"run", "--rm",
			"--mount", "type=bind,source="+dir+",target=/work,readonly",
			testAlpineImage,
			"cat", "/work/x",
		)

		if out != "via-mount" {
			t.Fatalf("output = %q", out)
		}
	})

	// Named volume must pass through untouched — the proxy only rewrites
	// absolute / relative-local paths, not named-volume sources.
	t.Run("NamedVolumePassthrough", func(t *testing.T) {
		volume := uniqueName("loop-int-vol")
		nameA := uniqueName("loop-int-vw")
		nameB := uniqueName("loop-int-vr")

		t.Cleanup(func() { env.removeContainer(nameA) })
		t.Cleanup(func() { env.removeContainer(nameB) })
		t.Cleanup(func() { env.dockerExec("volume", "rm", "-f", volume) })

		env.docker(t, "run", "--rm", "--name", nameA,
			"-v", volume+":/data",
			testAlpineImage,
			"sh", "-c", "echo persisted > /data/file",
		)

		out := env.docker(t, "run", "--rm", "--name", nameB,
			"-v", volume+":/data",
			testAlpineImage,
			"cat", "/data/file",
		)

		if out != "persisted" {
			t.Fatalf("output = %q", out)
		}
	})

	// Published container port should be reachable on the developer's
	// localhost via the proxy's PortForwarder hook.
	t.Run("PublishedPortForwarded", func(t *testing.T) {
		port, err := pickPort(0)

		if err != nil {
			t.Fatal(err)
		}

		name := uniqueName("loop-int-port")
		t.Cleanup(func() { env.removeContainer(name) })

		env.docker(t, "run", "-d", "--name", name,
			"-p", fmt.Sprintf("127.0.0.1:%d:80", port),
			testNginxImage,
		)

		url := fmt.Sprintf("http://127.0.0.1:%d/", port)
		deadline := time.Now().Add(60 * time.Second)

		var lastErr error

		for time.Now().Before(deadline) {
			resp, err := http.Get(url)

			if err == nil {
				resp.Body.Close()

				if resp.StatusCode == 200 {
					return
				}

				lastErr = fmt.Errorf("status %d", resp.StatusCode)
			} else {
				lastErr = err
			}

			time.Sleep(500 * time.Millisecond)
		}

		t.Fatalf("forwarded port not reachable: %v", lastErr)
	})
}
