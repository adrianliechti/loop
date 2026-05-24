package remotemount

import (
	"context"
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

	mu             sync.Mutex
	mounts         map[string]string
	ports          map[string]struct{}
	cancels        []context.CancelFunc
	nextRemotePort int
}

func NewManager(ctx context.Context, sshAddr, remoteRoot string) *Manager {
	ctx, cancel := context.WithCancel(ctx)

	return &Manager{
		ctx:            ctx,
		cancel:         cancel,
		sshAddr:        sshAddr,
		remoteRoot:     remoteRoot,
		mounts:         map[string]string{},
		ports:          map[string]struct{}{},
		nextRemotePort: 22000,
	}
}

func (m *Manager) Close() {
	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, cancel := range m.cancels {
		cancel()
	}
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
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("bind source %q does not exist on the local machine", source)
		}

		return "", err
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
	if localAddr == "" || localAddr == "0.0.0.0" || localAddr == "::" {
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

	go func() {
		err := client.Run(portCtx)

		m.mu.Lock()
		delete(m.ports, key)
		m.mu.Unlock()

		done <- err
	}()

	select {
	case <-ready:
		m.mu.Lock()
		m.cancels = append(m.cancels, cancel)
		m.mu.Unlock()
		return nil
	case err := <-done:
		cancel()
		m.mu.Lock()
		delete(m.ports, key)
		m.mu.Unlock()

		if err == nil {
			return portCtx.Err()
		}

		return err
	case <-ctx.Done():
		cancel()
		m.mu.Lock()
		delete(m.ports, key)
		m.mu.Unlock()
		return ctx.Err()
	}
}

func (m *Manager) mount(ctx context.Context, localRoot string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if remoteRoot, ok := m.mounts[localRoot]; ok {
		return remoteRoot, nil
	}

	sum := sha256.Sum256([]byte(localRoot))
	id := hex.EncodeToString(sum[:])[:16]
	remoteRoot := path.Join(m.remoteRoot, id)
	marker := path.Join("/tmp", "loop-mounted-"+id)

	sftpPort, err := system.FreePort(0)

	if err != nil {
		return "", err
	}

	mountCtx, cancel := context.WithCancel(m.ctx)

	if err := startSFTPServer(mountCtx, sftpPort, localRoot); err != nil {
		cancel()
		return "", err
	}

	remotePort := m.nextRemotePort
	m.nextRemotePort++

	cmd := fmt.Sprintf(
		"mkdir -p %s && sshfs -o allow_other -p %d root@localhost:/ %s && touch %s && /bin/sleep infinity",
		shellQuote(remoteRoot),
		remotePort,
		shellQuote(remoteRoot),
		shellQuote(marker),
	)

	client := ssh.New(m.sshAddr,
		ssh.WithRemotePortForward(ssh.PortForward{LocalPort: sftpPort, RemotePort: remotePort}),
		ssh.WithCommand(cmd),
		ssh.WithStderr(io.Discard),
		ssh.WithStdout(io.Discard),
	)

	go func() {
		if err := client.Run(mountCtx); err != nil && !errors.Is(err, context.Canceled) {
			log.Println("could not maintain remote mount", "path", localRoot, "error", err)
		}
	}()

	if err := m.waitForMarker(ctx, marker); err != nil {
		cancel()
		return "", err
	}

	m.cancels = append(m.cancels, cancel)
	m.mounts[localRoot] = remoteRoot
	return remoteRoot, nil
}

func (m *Manager) waitForMarker(ctx context.Context, marker string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	cmd := "test -f " + shellQuote(marker)

	for {
		client := ssh.New(m.sshAddr, ssh.WithCommand(cmd), ssh.WithStderr(io.Discard), ssh.WithStdout(io.Discard))

		if err := client.Run(ctx); err == nil {
			return nil
		}

		select {
		case <-ticker.C:
		case <-m.ctx.Done():
			return m.ctx.Err()
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for remote mount %q: %w", marker, ctx.Err())
		}
	}
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
