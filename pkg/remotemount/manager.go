package remotemount

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/adrianliechti/loop/pkg/sftp"
	"github.com/adrianliechti/loop/pkg/ssh"
	"github.com/adrianliechti/loop/pkg/system"
)

type Manager struct {
	ctx    context.Context
	cancel context.CancelFunc

	sshAddr    string
	remoteRoot string

	mu     sync.Mutex
	mounts map[string]*mountState
	ports  map[string]struct{}
}

// mountState tracks one localRoot's mount attempt so concurrent Resolve calls
// share a single attempt without holding the manager lock across remote I/O.
type mountState struct {
	ready chan struct{}

	id         string
	marker     string
	remoteRoot string
	err        error

	// cancel stops the mount's SFTP server and SSH sessions. On success it is
	// never called directly: the sessions must outlive establish() and end
	// with m.ctx, their parent.
	cancel context.CancelFunc
}

func NewManager(ctx context.Context, sshAddr, remoteRoot string) *Manager {
	ctx, cancel := context.WithCancel(ctx)

	return &Manager{
		ctx:        ctx,
		cancel:     cancel,
		sshAddr:    sshAddr,
		remoteRoot: remoteRoot,
		mounts:     map[string]*mountState{},
		ports:      map[string]struct{}{},
	}
}

func (m *Manager) Close() {
	m.mu.Lock()

	var states []*mountState

	for _, state := range m.mounts {
		states = append(states, state)
	}

	m.mu.Unlock()

	m.cancel()

	// Best effort: detach the sshfs mounts and remove the readiness markers on
	// the remote side so a later session doesn't inherit broken mountpoints.
	// The directories are left in place: containers created in this session
	// keep a valid (if empty) bind source across daemon restarts. The tunnel
	// may already be gone, hence the own context and ignored error.
	var parts []string

	for _, state := range states {
		select {
		case <-state.ready:
		default:
			continue
		}

		if state.err != nil {
			continue
		}

		parts = append(parts, cleanupCommand(state))
	}

	if len(parts) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := ssh.New(m.sshAddr,
		ssh.WithCommand(strings.Join(parts, "; ")+"; true"),
		ssh.WithStderr(io.Discard),
		ssh.WithStdout(io.Discard),
	)

	client.Run(ctx)
}

func cleanupCommand(state *mountState) string {
	dir := shellQuote(state.remoteRoot)
	marker := shellQuote(state.marker)

	return fmt.Sprintf("fusermount -uz %[1]s 2>/dev/null; umount -l %[1]s 2>/dev/null; rm -f %[2]s", dir, marker)
}

func (m *Manager) Resolve(ctx context.Context, source string) (string, error) {
	if !filepath.IsAbs(source) {
		return source, nil
	}

	abs, err := filepath.Abs(source)

	if err != nil {
		return "", err
	}

	info, err := os.Stat(abs)

	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}

		// Match dockerd's -v semantics: a missing bind source is created as a
		// directory instead of failing the container create.
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return "", fmt.Errorf("bind source %q does not exist and could not be created: %w", source, err)
		}

		info, err = os.Stat(abs)

		if err != nil {
			return "", err
		}
	}

	localRoot := abs
	remoteSuffix := ""

	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("bind source %q is not a regular file or directory", source)
		}

		localRoot = filepath.Dir(abs)
		remoteSuffix = filepath.Base(abs)
	}

	remoteRoot, err := m.mount(ctx, localRoot)

	if err != nil {
		return "", err
	}

	if remoteSuffix == "" {
		return remoteRoot, nil
	}

	return path.Join(remoteRoot, filepath.ToSlash(remoteSuffix)), nil
}

func (m *Manager) ForwardPort(ctx context.Context, localAddr string, localPort, remotePort int) error {
	// An unset host IP defaults to loopback; explicit wildcard or IPv6
	// requests are honored so `-p 0.0.0.0:x:y` is reachable from the LAN as
	// docker reports it.
	if localAddr == "" {
		localAddr = "127.0.0.1"
	}

	key := fmt.Sprintf("%s:%d:%d", localAddr, localPort, remotePort)

	m.mu.Lock()

	if _, ok := m.ports[key]; ok {
		m.mu.Unlock()
		return nil
	}

	m.ports[key] = struct{}{}
	m.mu.Unlock()

	ready := make(chan struct{})
	done := make(chan error, 1)
	portCtx, cancel := context.WithCancel(m.ctx)

	client := ssh.New(m.sshAddr,
		ssh.WithLocalPortForward(ssh.PortForward{LocalAddr: localAddr, LocalPort: localPort, RemotePort: remotePort}),
		ssh.WithReady(ready),
	)

	// The goroutine owns the key's lifecycle: it unregisters the port when the
	// forward ends, whatever the cause. Removing it earlier would let a retry
	// re-register while the old listener is still bound.
	go func() {
		err := client.Run(portCtx)
		cancel()

		m.mu.Lock()
		delete(m.ports, key)
		m.mu.Unlock()

		done <- err
	}()

	select {
	case <-ready:
		return nil
	case err := <-done:
		if err == nil {
			return portCtx.Err()
		}

		return err
	case <-ctx.Done():
		cancel()
		return ctx.Err()
	}
}

func (m *Manager) mount(ctx context.Context, localRoot string) (string, error) {
	m.mu.Lock()

	if state, ok := m.mounts[localRoot]; ok {
		m.mu.Unlock()

		select {
		case <-state.ready:
			return state.remoteRoot, state.err
		case <-ctx.Done():
			return "", ctx.Err()
		case <-m.ctx.Done():
			return "", m.ctx.Err()
		}
	}

	state := &mountState{ready: make(chan struct{})}
	m.mounts[localRoot] = state
	m.mu.Unlock()

	state.remoteRoot, state.err = m.establish(ctx, localRoot, state)

	if state.err != nil {
		// Drop the failed entry so a retry starts a fresh attempt instead of
		// reusing a poisoned one.
		m.mu.Lock()
		delete(m.mounts, localRoot)
		m.mu.Unlock()
	}

	close(state.ready)

	return state.remoteRoot, state.err
}

func (m *Manager) establish(ctx context.Context, localRoot string, state *mountState) (string, error) {
	// The remote directory is derived from the local path only, so the same
	// bind source maps to the same /data path across sessions and containers
	// created earlier keep a resolvable (if unmounted) source after restarts.
	sum := sha256.Sum256([]byte(localRoot))
	state.id = hex.EncodeToString(sum[:])[:16]
	remoteRoot := path.Join(m.remoteRoot, state.id)

	// The readiness marker is unique per attempt: a marker left behind by a
	// crashed session or a failed try must never satisfy a new wait.
	nonce := make([]byte, 4)
	rand.Read(nonce)
	state.marker = path.Join("/tmp", "loop-mounted-"+state.id+"-"+hex.EncodeToString(nonce))

	sftpPort, err := system.FreePort(0)

	if err != nil {
		return "", err
	}

	mountCtx, cancel := context.WithCancel(m.ctx)
	state.cancel = cancel

	if err := startSFTPServer(mountCtx, sftpPort, localRoot); err != nil {
		cancel()
		return "", err
	}

	// Let the remote sshd assign the reverse-forward port so concurrent
	// sessions attached to the same daemon never race for a fixed port.
	bound := make(chan int, 1)
	forwardReady := make(chan struct{})
	forwardDone := make(chan error, 1)

	forward := ssh.New(m.sshAddr,
		ssh.WithRemotePortForward(ssh.PortForward{LocalPort: sftpPort, RemotePort: 0, BoundRemotePort: bound}),
		ssh.WithReady(forwardReady),
	)

	go func() {
		err := forward.Run(mountCtx)

		if err != nil && !errors.Is(err, context.Canceled) {
			log.Println("could not maintain remote mount tunnel", "path", localRoot, "error", err)
		}

		forwardDone <- err
	}()

	select {
	case <-forwardReady:
	case err := <-forwardDone:
		cancel()

		if err == nil {
			err = mountCtx.Err()
		}

		return "", err
	case <-ctx.Done():
		cancel()
		return "", ctx.Err()
	}

	remotePort := <-bound

	// Detach any stale mountpoint first: a crashed session can leave a
	// disconnected sshfs mount on the (stable) directory that would otherwise
	// make every new mount fail with "Transport endpoint is not connected".
	cmd := fmt.Sprintf(
		"fusermount -uz %[1]s 2>/dev/null; umount -l %[1]s 2>/dev/null; mkdir -p %[1]s && sshfs -o allow_other -p %[2]d root@localhost:/ %[1]s && touch %[3]s && /bin/sleep infinity",
		shellQuote(remoteRoot),
		remotePort,
		shellQuote(state.marker),
	)

	client := ssh.New(m.sshAddr,
		ssh.WithCommand(cmd),
		ssh.WithStderr(io.Discard),
		ssh.WithStdout(io.Discard),
	)

	go func() {
		if err := client.Run(mountCtx); err != nil && !errors.Is(err, context.Canceled) {
			log.Println("could not maintain remote mount", "path", localRoot, "error", err)
		}
	}()

	if err := m.waitForMarker(ctx, state.marker); err != nil {
		cancel()

		// Best effort: unmount whatever the failed attempt left behind so a
		// retry (or the next session) starts from a clean mountpoint.
		state.remoteRoot = remoteRoot
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		cleanup := ssh.New(m.sshAddr,
			ssh.WithCommand(cleanupCommand(state)+"; true"),
			ssh.WithStderr(io.Discard),
			ssh.WithStdout(io.Discard),
		)

		cleanup.Run(cleanupCtx)

		return "", err
	}

	return remoteRoot, nil
}

// waitForMarker polls for the readiness marker with a single remote session
// instead of dialing a fresh SSH connection every tick.
func (m *Manager) waitForMarker(ctx context.Context, marker string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := fmt.Sprintf("i=0; while [ $i -lt 60 ]; do [ -f %s ] && exit 0; i=$((i+1)); sleep 0.5; done; exit 1", shellQuote(marker))

	client := ssh.New(m.sshAddr, ssh.WithCommand(cmd), ssh.WithStderr(io.Discard), ssh.WithStdout(io.Discard))

	if err := client.Run(ctx); err != nil {
		return fmt.Errorf("timed out waiting for remote mount %q: %w", marker, err)
	}

	return nil
}

func startSFTPServer(ctx context.Context, port int, root string) error {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	server, err := sftp.NewServer(addr, root)

	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", addr)

	if err != nil {
		server.Close()
		return err
	}

	go func() {
		<-ctx.Done()
		listener.Close()
		server.Close()
	}()

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Println("could not serve sftp", "error", err)
		}
	}()

	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
