package tunnel

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseTargets(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		forwards, err := ParseTargets([]string{"db.internal:5432", "[::1]:6379"})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(forwards) != 2 {
			t.Fatalf("want 2 forwards, got %d", len(forwards))
		}

		if forwards[0].RemoteAddr != "db.internal" || forwards[0].RemotePort != 5432 {
			t.Errorf("target 0: got %s:%d", forwards[0].RemoteAddr, forwards[0].RemotePort)
		}

		// Local listen port mirrors the remote port.
		if forwards[0].LocalPort != 5432 || forwards[0].LocalAddr != "127.0.0.1" {
			t.Errorf("local 0: got %s:%d", forwards[0].LocalAddr, forwards[0].LocalPort)
		}

		if forwards[1].RemoteAddr != "::1" || forwards[1].RemotePort != 6379 {
			t.Errorf("target 1: got %s:%d", forwards[1].RemoteAddr, forwards[1].RemotePort)
		}
	})

	t.Run("duplicate ports", func(t *testing.T) {
		if _, err := ParseTargets([]string{"db1.internal:5432", "db2.internal:5432"}); err == nil {
			t.Error("expected error for colliding local ports, got nil")
		}
	})

	invalid := []string{
		"db.internal",   // no port
		":5432",         // no host
		"db.internal:0", // port out of range
		"db:99999",      // port out of range
		"db:http",       // non-numeric port
	}

	for _, arg := range invalid {
		t.Run(arg, func(t *testing.T) {
			if _, err := ParseTargets([]string{arg}); err == nil {
				t.Errorf("expected error for %q, got nil", arg)
			}
		})
	}
}

func TestLimitedBuffer(t *testing.T) {
	var b limitedBuffer

	n, err := b.Write(bytes.Repeat([]byte("x"), 4096))

	if err != nil || n != 4096 {
		t.Fatalf("want (4096, nil), got (%d, %v)", n, err)
	}

	if got := len(b.String()); got != 2048 {
		t.Errorf("want 2048 bytes retained, got %d", got)
	}
}

// TestProxyCommand runs the generated script against a real local listener and
// checks that data flows both ways: the test writes ping to the proxy's stdin,
// a TCP server answers pong, and the reply must appear on the proxy's stdout.
func TestProxyCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}

	t.Run("host tools ipv4", func(t *testing.T) {
		runProxy(t, "127.0.0.1:0", "127.0.0.1", "")
	})

	t.Run("host tools ipv6", func(t *testing.T) {
		runProxy(t, "[::1]:0", "::1", "")
	})

	// Restrict PATH to bash and cat only, forcing the /dev/tcp fallback.
	t.Run("bash only", func(t *testing.T) {
		dir := t.TempDir()

		for _, tool := range []string{"bash", "cat"} {
			path, err := exec.LookPath(tool)

			if err != nil {
				t.Skipf("%s not found: %v", tool, err)
			}

			if err := os.Symlink(path, filepath.Join(dir, tool)); err != nil {
				t.Fatal(err)
			}
		}

		runProxy(t, "127.0.0.1:0", "127.0.0.1", dir)
	})

	t.Run("no tools", func(t *testing.T) {
		argv := proxyCommand("127.0.0.1", 1)

		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Env = []string{"PATH=" + t.TempDir()}

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		err := cmd.Run()

		var exitErr *exec.ExitError

		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 127 {
			t.Fatalf("want exit code 127, got %v", err)
		}

		if !strings.Contains(stderr.String(), "no socat, nc, or bash") {
			t.Errorf("stderr missing diagnostic, got %q", stderr.String())
		}
	})
}

// runProxy starts a one-shot ping/pong TCP server on listenAddr and drives
// proxyCommand(host, port) against it. An empty path keeps the host's PATH;
// otherwise PATH is replaced to force a specific fallback branch.
func runProxy(t *testing.T, listenAddr, host, path string) {
	t.Helper()

	listener, err := net.Listen("tcp", listenAddr)

	if err != nil {
		t.Skipf("cannot listen on %s: %v", listenAddr, err)
	}

	defer listener.Close()

	go func() {
		conn, err := listener.Accept()

		if err != nil {
			return
		}

		defer conn.Close()

		if _, err := bufio.NewReader(conn).ReadString('\n'); err != nil {
			return
		}

		conn.Write([]byte("pong\n"))
	}()

	argv := proxyCommand(host, listener.Addr().(*net.TCPAddr).Port)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)

	if path != "" {
		cmd.Env = []string{"PATH=" + path}
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdin, err := cmd.StdinPipe()

	if err != nil {
		t.Fatal(err)
	}

	stdout, err := cmd.StdoutPipe()

	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	io.WriteString(stdin, "ping\n")

	reply, err := bufio.NewReader(stdout).ReadString('\n')

	if err != nil {
		t.Fatalf("read reply: %v (stderr: %q)", err, stderr.String())
	}

	if reply != "pong\n" {
		t.Fatalf("want pong, got %q", reply)
	}

	stdin.Close()
	cmd.Wait()
}
