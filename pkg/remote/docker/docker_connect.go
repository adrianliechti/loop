package docker

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/pkg/dockerproxy"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/remotemount"
	"github.com/adrianliechti/loop/pkg/ssh"
)

type ConnectOptions struct {
	Namespace string
}

func Connect(ctx context.Context, client kubernetes.Client, name string, options *ConnectOptions) error {
	if options == nil {
		options = new(ConnectOptions)
	}

	if options.Namespace == "" {
		options.Namespace = client.Namespace()
	}

	cli.Infof("★ Connecting to Docker instance '%s'", name)

	podName := resourceName(name) + "-0"

	if _, err := client.WaitForPod(ctx, options.Namespace, podName); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	proxyPort, err := pickPort(2375)

	if err != nil {
		return err
	}

	daemonPort, err := pickPort(0)

	if err != nil {
		return err
	}

	sshPort, err := pickPort(0)

	if err != nil {
		return err
	}

	sshForwardReady := make(chan struct{})
	sshForwardDone := make(chan error, 1)

	go func() {
		sshForwardDone <- client.PodPortForward(ctx, options.Namespace, podName, "127.0.0.1", map[int]int{sshPort: 22}, sshForwardReady)
		cancel()
	}()

	if err := waitForReady(ctx, sshForwardReady, sshForwardDone); err != nil {
		return err
	}

	sshAddr := fmt.Sprintf("127.0.0.1:%d", sshPort)
	daemonReady := make(chan struct{})
	daemonDone := make(chan error, 1)

	go func() {
		sshClient := ssh.New(sshAddr,
			ssh.WithLocalPortForward(ssh.PortForward{LocalPort: daemonPort, RemotePort: 2375}),
			ssh.WithReady(daemonReady),
		)

		daemonDone <- sshClient.Run(ctx)
		cancel()
	}()

	if err := waitForReady(ctx, daemonReady, daemonDone); err != nil {
		return err
	}

	if err := waitForDocker(ctx, daemonPort); err != nil {
		return err
	}

	mounts := remotemount.NewManager(ctx, sshAddr, "/data")
	defer mounts.Close()

	proxyReady := make(chan struct{})
	proxyDone := make(chan error, 1)

	go func() {
		proxyDone <- dockerproxy.Serve(ctx, fmt.Sprintf("127.0.0.1:%d", proxyPort), fmt.Sprintf("http://127.0.0.1:%d", daemonPort), mounts, proxyReady)
		cancel()
	}()

	if err := waitForReady(ctx, proxyReady, proxyDone); err != nil {
		return err
	}

	docker := "docker"
	loopContext := "loop-" + name

	val, err := exec.Command(docker, "context", "show").Output()

	if err != nil {
		return fmt.Errorf("could not get current Docker context: %w", err)
	}

	currentContext := strings.TrimSpace(string(val))

	defer func() {
		cli.Info("★ Resetting Docker context to '" + currentContext + "'")

		if currentContext != loopContext {
			runDocker(docker, "context", "use", currentContext)
		} else {
			runDocker(docker, "context", "use", "default")
		}

		runDocker(docker, "context", "rm", "-f", loopContext)
	}()

	cli.Info("★ Setting Docker context to '" + loopContext + "'")

	if currentContext == loopContext {
		runDocker(docker, "context", "use", "default")
	}

	runDocker(docker, "context", "rm", "-f", loopContext)

	if err := runDocker(docker, "context", "create", loopContext, "--docker", fmt.Sprintf("host=tcp://127.0.0.1:%d", proxyPort)); err != nil {
		return err
	}

	if err := runDocker(docker, "context", "use", loopContext); err != nil {
		return err
	}

	cli.Info("★ Press Ctrl+C to disconnect")

	select {
	case err := <-sshForwardDone:
		return errOrContext(ctx, err)
	case err := <-daemonDone:
		return errOrContext(ctx, err)
	case err := <-proxyDone:
		return errOrContext(ctx, err)
	case <-ctx.Done():
		return nil
	}
}

// pickPort returns a free local TCP port, preferring `preference` if available.
// There is an inherent TOCTOU window between picking and binding; callers must
// tolerate the chosen port being claimed by another process before they bind.
func pickPort(preference int) (int, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preference))

	if err != nil && preference != 0 {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
	}

	if err != nil {
		return 0, err
	}

	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitForReady(ctx context.Context, ready <-chan struct{}, done <-chan error) error {
	select {
	case <-ready:
		return nil
	case err := <-done:
		return errOrContext(ctx, err)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func errOrContext(ctx context.Context, err error) error {
	if err != nil {
		return err
	}

	return ctx.Err()
}

func waitForDocker(ctx context.Context, port int) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client := &http.Client{
		Timeout: 500 * time.Millisecond,
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/_ping", port)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

		if err != nil {
			return err
		}

		resp, err := client.Do(req)

		if err == nil {
			resp.Body.Close()

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for Docker daemon: %w", ctx.Err())
		}
	}
}

func runDocker(docker string, args ...string) error {
	cmd := exec.Command(docker, args...)

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}

	return nil
}
