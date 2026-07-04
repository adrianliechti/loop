package docker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/pkg/dockerproxy"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/remotemount"
	"github.com/adrianliechti/loop/pkg/ssh"
	"github.com/adrianliechti/loop/pkg/system"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// Fail fast on unknown names: waiting for a pod of a daemon that was never
	// created would block forever.
	if _, err := client.AppsV1().StatefulSets(options.Namespace).Get(ctx, resourceName(name), metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("docker instance %q not found in namespace %q", name, options.Namespace)
		}

		return err
	}

	podName := resourceName(name) + "-0"

	if _, err := client.WaitForPod(ctx, options.Namespace, podName); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	proxyPort, err := system.FreePort(2375)

	if err != nil {
		return err
	}

	daemonPort, err := system.FreePort(0)

	if err != nil {
		return err
	}

	sshPort, err := system.FreePort(0)

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

	// The context name carries the proxy port so concurrent sessions against
	// the same instance each own a distinct context instead of force-removing
	// each other's.
	loopContext := fmt.Sprintf("loop-%s-%d", name, proxyPort)

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
		// A failing goroutine sends its error before cancelling ctx, so both
		// cases may be ready and Go picks randomly. Drain the channels so a
		// real failure isn't reported as a clean shutdown.
		return drainErrors(sshForwardDone, daemonDone, proxyDone)
	}
}

// drainErrors returns the first real error already buffered in the given
// channels, ignoring plain cancellation. Non-blocking: on a user-initiated
// shutdown the goroutines may not have reported (anything) yet.
func drainErrors(channels ...<-chan error) error {
	for _, ch := range channels {
		select {
		case err := <-ch:
			if err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
		default:
		}
	}

	return nil
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
