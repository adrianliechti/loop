package tunnel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/ssh"
	"github.com/adrianliechti/loop/pkg/system"

	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const tunnelImage = "ghcr.io/adrianliechti/loop-tunnel"

type RunOptions struct {
	Namespace string
	Image     string

	// Pod, if set, tunnels through an existing pod via exec instead of creating
	// a dedicated jump pod. Container selects which of its containers to exec in.
	Pod       string
	Container string

	Forwards []ssh.PortForward
}

// Run forwards the local side of each configured forward to hosts reachable
// from the cluster and blocks until ctx is cancelled or the tunnel fails.
func Run(ctx context.Context, client kubernetes.Client, options *RunOptions) error {
	if options == nil {
		options = new(RunOptions)
	}

	if options.Namespace == "" {
		options.Namespace = client.Namespace()
	}

	if options.Image == "" {
		options.Image = tunnelImage
	}

	if len(options.Forwards) == 0 {
		return errors.New("at least one port forward is required")
	}

	if options.Pod != "" {
		return runViaExec(ctx, client, options)
	}

	return runViaPod(ctx, client, options)
}

// runViaPod spins up a dedicated loop-tunnel (sshd) pod and drives local
// port forwards through it. It reaches whatever is routable from the namespace:
// cluster IPs, other pods, and egress destinations.
func runViaPod(ctx context.Context, client kubernetes.Client, options *RunOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	podName := "loop-tunnel-" + uuid.NewString()[0:7]

	cli.Infof("★ creating tunnel pod %s/%s...", options.Namespace, podName)

	if err := createPod(ctx, client, options.Namespace, podName, options.Image); err != nil {
		return err
	}

	defer func() {
		cli.Infof("★ removing tunnel pod %s/%s...", options.Namespace, podName)

		if err := deletePod(context.Background(), client, options.Namespace, podName); err != nil {
			cli.Warnf("★ failed to remove tunnel pod %s/%s: %v", options.Namespace, podName, err)
		}
	}()

	if _, err := client.WaitForPod(ctx, options.Namespace, podName); err != nil {
		return err
	}

	sshPort, err := system.FreePort(0)

	if err != nil {
		return err
	}

	forwardReady := make(chan struct{})
	forwardDone := make(chan error, 1)

	go func() {
		forwardDone <- client.PodPortForward(ctx, options.Namespace, podName, "127.0.0.1", map[int]int{sshPort: 22}, forwardReady)
		cancel()
	}()

	select {
	case <-forwardReady:
	case err := <-forwardDone:
		return errOrContext(ctx, err)
	case <-ctx.Done():
		return ctx.Err()
	}

	sshOptions := make([]ssh.Option, 0, len(options.Forwards)+1)

	for _, f := range options.Forwards {
		sshOptions = append(sshOptions, ssh.WithLocalPortForward(f))
	}

	sshReady := make(chan struct{})
	sshOptions = append(sshOptions, ssh.WithReady(sshReady))

	sshClient := ssh.New(fmt.Sprintf("127.0.0.1:%d", sshPort), sshOptions...)

	sshDone := make(chan error, 1)

	go func() {
		sshDone <- sshClient.Run(ctx)
		cancel()
	}()

	select {
	case <-sshReady:
	case err := <-sshDone:
		return errOrContext(ctx, err)
	case err := <-forwardDone:
		return errOrContext(ctx, err)
	case <-ctx.Done():
		return ctx.Err()
	}

	printForwards(options, podName)

	select {
	case err := <-forwardDone:
		return errOrContext(ctx, err)
	case err := <-sshDone:
		return errOrContext(ctx, err)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// runViaExec tunnels through an existing pod without adding anything to it:
// each accepted connection execs a small proxy in the pod that dials the target
// from the pod's network namespace. It depends on socat, nc, or bash being
// present in the target image.
func runViaExec(ctx context.Context, client kubernetes.Client, options *RunOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Fail fast on a wrong pod name and resolve the default container once,
	// rather than per accepted connection.
	pod, err := client.CoreV1().Pods(options.Namespace).Get(ctx, options.Pod, metav1.GetOptions{})

	if err != nil {
		return err
	}

	if options.Container == "" {
		options.Container = pod.Spec.Containers[0].Name
	}

	errs := make(chan error, len(options.Forwards))

	for _, f := range options.Forwards {
		listener, err := net.Listen("tcp", net.JoinHostPort(f.LocalAddr, strconv.Itoa(f.LocalPort)))

		if err != nil {
			return err
		}

		defer listener.Close()

		go func(listener net.Listener, f ssh.PortForward) {
			errs <- serveExec(ctx, client, options, listener, f)
		}(listener, f)
	}

	printForwards(options, options.Pod)

	select {
	case err := <-errs:
		return errOrContext(ctx, err)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func serveExec(ctx context.Context, client kubernetes.Client, options *RunOptions, listener net.Listener, f ssh.PortForward) error {
	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	command := proxyCommand(f.RemoteAddr, f.RemotePort)

	for {
		conn, err := listener.Accept()

		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return err
		}

		go func() {
			defer conn.Close()

			var stderr limitedBuffer

			if err := client.PodExec(ctx, options.Namespace, options.Pod, options.Container, command, false, conn, conn, &stderr); err != nil && ctx.Err() == nil {
				message := strings.TrimSpace(stderr.String())

				if message == "" {
					message = err.Error()
				}

				cli.Warnf("★ tunnel %s:%d: %s", f.RemoteAddr, f.RemotePort, message)
			}
		}()
	}
}

// limitedBuffer keeps the first bytes of remote stderr for diagnostics without
// letting a chatty stream grow per-connection memory.
type limitedBuffer struct {
	buf bytes.Buffer
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if free := 2048 - b.buf.Len(); free > 0 {
		if len(p) > free {
			b.buf.Write(p[:free])
		} else {
			b.buf.Write(p)
		}
	}

	return len(p), nil
}

func (b *limitedBuffer) String() string {
	return b.buf.String()
}

// proxyCommand pipes stdio to host:port from inside the pod. It prefers socat,
// falls back to nc, and finally to bash's /dev/tcp (a bash builtin, so it needs
// bash — dash/ash do not support it). Host and port are passed as positional
// args so they are never interpolated into a shell string. IPv6 literals must
// be bracketed for socat but passed bare to nc and /dev/tcp.
//
// In the bash fallback, kill reaps the socket-reader cat once the local side
// closes stdin; if the remote closes first, the session lingers until the
// local client hangs up — /dev/tcp has no half-close.
func proxyCommand(host string, port int) []string {
	script := `if command -v socat >/dev/null 2>&1; then
  case "$1" in
  *:*) exec socat STDIO "TCP:[$1]:$2" ;;
  *) exec socat STDIO "TCP:$1:$2" ;;
  esac
fi
if command -v nc >/dev/null 2>&1; then exec nc "$1" "$2"; fi
if command -v bash >/dev/null 2>&1; then exec bash -c 'exec 3<>"/dev/tcp/$1/$2"; cat <&3 & cat >&3; kill $! 2>/dev/null' bash "$1" "$2"; fi
echo "tunnel: target container has no socat, nc, or bash" >&2
exit 127`

	return []string{"sh", "-c", script, "sh", host, strconv.Itoa(port)}
}

func printForwards(options *RunOptions, via string) {
	for _, f := range options.Forwards {
		cli.Infof("★ forwarding tcp://%s:%d => tcp://%s:%d (via %s)", f.LocalAddr, f.LocalPort, f.RemoteAddr, f.RemotePort, via)
	}

	cli.Info("★ Press Ctrl+C to disconnect")
}

func createPod(ctx context.Context, client kubernetes.Client, namespace, name, image string) error {
	labels := map[string]string{
		"app.kubernetes.io/name":     "loop-tunnel",
		"app.kubernetes.io/instance": name,
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},

		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "tunnel",
					Image: image,
				},
			},
		},
	}

	_, err := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})

	return err
}

func deletePod(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	if err := client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !kubernetes.IsNotFound(err) {
		return err
	}

	return nil
}

// ParseTargets parses host:port arguments into port forwards: listen locally
// on 127.0.0.1 with the same port number and dial host:port from the pod.
func ParseTargets(args []string) ([]ssh.PortForward, error) {
	var result []ssh.PortForward

	seen := map[int]string{}

	for _, arg := range args {
		host, portStr, err := net.SplitHostPort(arg)

		if err != nil {
			return nil, fmt.Errorf("invalid target %q: want host:port", arg)
		}

		if host == "" {
			return nil, fmt.Errorf("invalid target %q: host is empty", arg)
		}

		port, err := parsePort(portStr)

		if err != nil {
			return nil, fmt.Errorf("invalid target %q: %w", arg, err)
		}

		if prev, ok := seen[port]; ok {
			return nil, fmt.Errorf("targets %q and %q both need local port %d", prev, arg, port)
		}

		seen[port] = arg

		result = append(result, ssh.PortForward{
			LocalAddr: "127.0.0.1",
			LocalPort: port,

			RemoteAddr: host,
			RemotePort: port,
		})
	}

	return result, nil
}

func parsePort(s string) (int, error) {
	port, err := strconv.Atoi(s)

	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("invalid port %q", s)
	}

	return port, nil
}

func errOrContext(ctx context.Context, err error) error {
	if err != nil {
		return err
	}

	return ctx.Err()
}
