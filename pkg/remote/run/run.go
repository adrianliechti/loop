package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/pkg/docker"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/ssh"
	"github.com/adrianliechti/loop/pkg/system"

	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Container struct {
	Image string

	Identity *Identity

	TTY bool

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	Ports   []Port
	Volumes []Volume
}

type Identity struct {
	UID int
	GID int
}

type Port struct {
	Source int
	Target int
}

type Volume struct {
	Source string
	Target string

	Identity *Identity
}

type SyncMode string

const (
	SyncModeNone  SyncMode = ""
	SyncModeMount SyncMode = "mount"
)

type RunOptions struct {
	Name      string
	Namespace string

	// Loop Tunnel image overwrite
	Image string

	SyncMode SyncMode

	OnPod    func(ctx context.Context, client kubernetes.Client, pod *corev1.Pod) error
	OnReady  func(ctx context.Context, client kubernetes.Client, pod *corev1.Pod) error
	OnDelete func(ctx context.Context, client kubernetes.Client, pod *corev1.Pod) error
}

func Run(ctx context.Context, client kubernetes.Client, container *Container, options *RunOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if options == nil {
		options = new(RunOptions)
	}

	if options.Name == "" {
		options.Name = "loop-run-" + uuid.NewString()[0:7]
	}

	if options.Namespace == "" {
		options.Namespace = client.Namespace()
	}

	if options.Image == "" {
		options.Image = "ghcr.io/adrianliechti/loop-tunnel"
	}

	if options.SyncMode == SyncModeMount && len(container.Volumes) > 1 {
		return errors.New("mount mode currently only supports a single volume")
	}

	pod := templatePod(container, options)

	if options.OnPod != nil {
		if err := options.OnPod(ctx, client, pod); err != nil {
			return err
		}
	}

	cli.Infof("★ creating container (%s/%s)...", pod.Namespace, pod.Name)

	defer func() {
		cli.Infof("★ removing container (%s/%s)...", pod.Namespace, pod.Name)
		stopPod(context.Background(), client, pod.Namespace, pod.Name)

		if options.OnDelete != nil {
			options.OnDelete(ctx, client, pod)
		}
	}()

	if err := startPod(ctx, client, pod); err != nil {
		return err
	}

	if options.SyncMode != SyncModeMount {
		cli.Infof("★ copying volumes data...")

		if err := copyVolumes(ctx, client, pod.Namespace, pod.Name, container.Volumes); err != nil {
			return err
		}
	}

	// connectTunnel only returns non-nil before signalling ready, so we can
	// reliably surface setup errors via tunnelDone. Late errors (after ready)
	// are logged from inside the SSH/port-forward goroutines.
	tunnelReady := make(chan struct{})
	tunnelDone := make(chan error, 1)

	go func() {
		tunnelDone <- connectTunnel(ctx, client, pod, container, options, tunnelReady)
		cancel()
	}()

	// Wait for tunnels to actually be set up before firing OnReady, so
	// callbacks like "open browser at localhost:port" don't race SSH listen.
	//
	// connectTunnel only returns non-nil before signalling ready, so a nil
	// from tunnelDone here means it raced with ready/ctx — fall back to
	// ctx.Err() rather than silently returning success.
	select {
	case <-tunnelReady:
	case err := <-tunnelDone:
		if err == nil {
			return ctx.Err()
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}

	if options.OnReady != nil {
		if err := options.OnReady(ctx, client, pod); err != nil {
			return err
		}
	}

	return client.PodAttach(ctx, pod.Namespace, pod.Name, "main", container.TTY, container.Stdin, container.Stdout, container.Stderr)
}

func templatePod(container *Container, options *RunOptions) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.Name,
			Namespace: options.Namespace,
		},

		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",

					Image:           container.Image,
					ImagePullPolicy: corev1.PullAlways,

					TTY:   container.TTY,
					Stdin: container.Stdin != nil,
				},
			},

			TerminationGracePeriodSeconds: kubernetes.Ptr(int64(10)),
		},
	}

	if len(container.Volumes) == 0 && len(container.Ports) == 0 {
		return pod
	}

	volume := corev1.Volume{
		Name: "loop-data",

		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}

	pod.Spec.Volumes = append(pod.Spec.Volumes, volume)

	var mounts []corev1.VolumeMount

	for _, v := range container.Volumes {
		mount := corev1.VolumeMount{
			Name: volume.Name,

			MountPath: v.Target,
			SubPath:   strings.TrimLeft(v.Target, "/"),
		}

		if options.SyncMode == SyncModeMount {
			mount.MountPropagation = kubernetes.Ptr(corev1.MountPropagationHostToContainer)
		}

		mounts = append(mounts, mount)
	}

	for i, c := range pod.Spec.InitContainers {
		pod.Spec.InitContainers[i].VolumeMounts = append(c.VolumeMounts, mounts...)
	}

	for i, c := range pod.Spec.Containers {
		pod.Spec.Containers[i].VolumeMounts = append(c.VolumeMounts, mounts...)
	}

	tunnel := corev1.Container{
		Name: "loop-tunnel",

		Image:           options.Image,
		ImagePullPolicy: corev1.PullAlways,

		SecurityContext: &corev1.SecurityContext{},

		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},

			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		},

		VolumeMounts: []corev1.VolumeMount{
			{
				Name: volume.Name,

				MountPath: "/data",
			},
		},
	}

	if options.SyncMode == SyncModeMount {
		tunnel.SecurityContext.Privileged = kubernetes.Ptr(true)
		tunnel.VolumeMounts[0].MountPropagation = kubernetes.Ptr(corev1.MountPropagationBidirectional)
	}

	pod.Spec.Containers = append(pod.Spec.Containers, tunnel)

	return pod
}

func startPod(ctx context.Context, client kubernetes.Client, pod *corev1.Pod) error {
	pod, err := client.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})

	if err != nil {
		return err
	}

	if _, err := client.WaitForPod(ctx, pod.Namespace, pod.Name); err != nil {
		return err
	}

	return nil
}

func stopPod(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	if err := client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: kubernetes.Ptr(int64(0)),
	}); err != nil && !kubernetes.IsNotFound(err) {
		return err
	}

	return nil
}

func connectTunnel(ctx context.Context, client kubernetes.Client, pod *corev1.Pod, container *Container, options *RunOptions, ready chan<- struct{}) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	port, err := system.FreePort(0)

	if err != nil {
		return err
	}

	pfReady := make(chan struct{})
	pfErr := make(chan error, 1)

	go func() {
		pfErr <- client.PodPortForward(ctx, pod.Namespace, pod.Name, "", map[int]int{port: 22}, pfReady)
		cancel()
	}()

	// Wait for the port-forward to be ready, but bail out if it fails first
	// (otherwise an early port-forward error leaves us blocked here forever).
	// A nil pfErr means PodPortForward returned cleanly only because ctx was
	// cancelled, so surface that as the cause.
	select {
	case <-pfReady:
	case err := <-pfErr:
		if err == nil {
			return ctx.Err()
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)

	// portsReady fires once the local forwarded ports are actually listening;
	// this is what OnReady needs to wait for (e.g. opening a browser at a
	// forwarded port). nil when there are no forwarded ports.
	var portsReady <-chan struct{}

	if len(container.Ports) > 0 {
		ch := make(chan struct{})
		portsReady = ch

		go func() {
			if err := tunnelPorts(ctx, addr, container.Ports, ch); err != nil {
				cli.Error(err)
			}

			cancel()
		}()
	}

	if len(container.Volumes) > 0 && options.SyncMode == SyncModeMount {
		volume := container.Volumes[0]

		sftpport, err := system.FreePort(0)

		if err != nil {
			return err
		}

		if err := startServer(ctx, sftpport, volume.Source); err != nil {
			return err
		}

		targetPath := path.Join("/data", volume.Target)
		marker := "/tmp/loop-mounted"

		// Shell-quote the target path: it is user-supplied (via `-v src:tgt`)
		// and gets concatenated into a shell command on the remote side.
		// Mounting succeeds first, then the marker is touched — so the marker
		// is also the signal that sshfs has actually mounted (see below).
		cmd := fmt.Sprintf(
			"sshfs -o allow_other -p 2222 root@localhost:/ %s && touch %s && /bin/sleep infinity",
			shellQuote(targetPath),
			shellQuote(marker),
		)

		c := ssh.New(addr,
			ssh.WithRemotePortForward(ssh.PortForward{LocalPort: sftpport, RemotePort: 2222}),
			ssh.WithCommand(cmd),
			ssh.WithStderr(os.Stderr),
			ssh.WithStdout(os.Stdout),
		)

		go func() {
			if err := c.Run(ctx); err != nil {
				cli.Error(err)
			}

			cancel()
		}()

		// Wait for sshfs to actually mount before letting OnReady/attach
		// proceed against an unmounted workspace. waitForMount polls the
		// marker that the command above touches after a successful mount.
		if err := waitForMount(ctx, client, pod.Namespace, pod.Name, "loop-tunnel", marker); err != nil {
			return err
		}
	}

	if portsReady != nil {
		select {
		case <-portsReady:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	close(ready)

	<-ctx.Done()

	return nil
}

// shellQuote single-quotes s for safe inclusion in a /bin/sh command line.
// Single quotes inside s are escaped as '\''.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// waitForMount polls the pod for marker until it exists (sshfs has mounted)
// or the context expires. Each PodExec call has non-trivial overhead, so
// the poll interval is deliberately coarse.
func waitForMount(ctx context.Context, client kubernetes.Client, namespace, pod, container, marker string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		if err := client.PodExec(ctx, namespace, pod, container, []string{"test", "-f", marker}, false, nil, io.Discard, io.Discard); err == nil {
			return nil
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for sshfs mount: %w", ctx.Err())
		}
	}
}

func tunnelPorts(ctx context.Context, addr string, ports []Port, ready chan<- struct{}) error {
	options := []ssh.Option{ssh.WithReady(ready)}

	for _, p := range ports {
		options = append(options, ssh.WithLocalPortForward(ssh.PortForward{LocalPort: p.Source, RemotePort: p.Target}))
	}

	client := ssh.New(addr, options...)

	return client.Run(ctx)
}

func copyVolumes(ctx context.Context, client kubernetes.Client, namespace, name string, volumes []Volume) error {
	for _, v := range volumes {
		if err := copyVolume(ctx, client, namespace, name, v); err != nil {
			return err
		}
	}

	return nil
}

func copyVolume(ctx context.Context, client kubernetes.Client, namespace, name string, v Volume) error {
	target := path.Join("/data", v.Target)

	options := &docker.TarballOptions{}

	if v.Identity != nil {
		options.UID = &v.Identity.UID
		options.GID = &v.Identity.GID
	}

	r, w := io.Pipe()

	// Closing the reader on exit causes a hung WriteTarball (no consumer left)
	// to fail with ErrClosedPipe, releasing the goroutine.
	defer r.Close()

	go func() {
		err := docker.WriteTarball(w, v.Source, options)
		w.CloseWithError(err)
	}()

	return client.PodExec(ctx, namespace, name, "loop-tunnel", []string{"tar", "xf", "-", "-C", target}, false, r, os.Stdout, os.Stdout)
}
