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

	ports, releasePorts, err := reserveLocalPorts(2375, 0, 0)

	if err != nil {
		return err
	}

	proxyPort := ports[0]
	daemonPort := ports[1]
	sshPort := ports[2]
	defer releasePorts()

	releasePorts()

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

func reserveLocalPorts(preferences ...int) ([]int, func(), error) {
	listeners := make([]net.Listener, 0, len(preferences))
	closed := false

	cleanup := func() {
		if closed {
			return
		}

		closed = true

		for _, listener := range listeners {
			listener.Close()
		}
	}

	for _, preference := range preferences {
		listener, err := reserveLocalPort(preference)

		if err != nil {
			cleanup()
			return nil, nil, err
		}

		listeners = append(listeners, listener)
	}

	ports := make([]int, 0, len(listeners))

	for _, listener := range listeners {
		ports = append(ports, listener.Addr().(*net.TCPAddr).Port)
	}

	return ports, cleanup, nil
}

func reserveLocalPort(preference int) (net.Listener, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preference))

	if err == nil || preference == 0 {
		return listener, err
	}

	return net.Listen("tcp", "127.0.0.1:0")
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
