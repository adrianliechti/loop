package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/ssh"
	"github.com/adrianliechti/loop/pkg/system"
	"github.com/adrianliechti/loop/pkg/to"

	"github.com/google/uuid"

	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/idtools"

	corev1 "k8s.io/api/core/v1"
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
	// SyncModeLocal  SyncMode = "local"
	// SyncModeRemote SyncMode = "remote"
)

type RunOptions struct {
	Name      string
	Namespace string

	// Loop Tunnel image overwrite
	Image string

	SyncMode SyncMode

	OnPod   func(ctx context.Context, client kubernetes.Client, pod *corev1.Pod) error
	OnReady func(ctx context.Context, client kubernetes.Client, pod *corev1.Pod) error
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

	if err := startPod(ctx, client, pod); err != nil {
		return err
	}

	defer func() {
		cli.Infof("★ removing container (%s/%s)...", pod.Namespace, pod.Name)
		stopPod(context.Background(), client, pod.Namespace, pod.Name)
	}()

	if options.SyncMode != SyncModeMount {
		cli.Infof("★ copying volumes data...")

		if err := copyVolumes(ctx, client, pod.Namespace, pod.Name, container.Volumes); err != nil {
			return err
		}
	}

	go func() {
		connectTunnel(ctx, client, pod, container, options)
		cancel()
	}()

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

			TerminationGracePeriodSeconds: to.Ptr(int64(10)),
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
			mount.MountPropagation = to.Ptr(corev1.MountPropagationHostToContainer)
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

		VolumeMounts: []corev1.VolumeMount{
			{
				Name: volume.Name,

				MountPath: "/data",
			},
		},
	}

	if options.SyncMode == SyncModeMount {
		tunnel.SecurityContext.Privileged = to.Ptr(true)
		tunnel.VolumeMounts[0].MountPropagation = to.Ptr(corev1.MountPropagationBidirectional)
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
	return client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: to.Ptr(int64(0)),
	})
}

func connectTunnel(ctx context.Context, client kubernetes.Client, pod *corev1.Pod, container *Container, options *RunOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	port, err := system.FreePort(0)

	if err != nil {
		return err
	}

	ready := make(chan struct{})

	go func() {
		client.PodPortForward(ctx, pod.Namespace, pod.Name, "", map[int]int{port: 22}, ready)
		cancel()
	}()

	<-ready

	addr := fmt.Sprintf("127.0.0.1:%d", port)

	if len(container.Ports) > 0 {
		go func() {
			if err := tunnelPorts(ctx, addr, container.Ports); err != nil {
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

		options := []ssh.Option{
			ssh.WithRemotePortForward(ssh.PortForward{LocalPort: sftpport, RemotePort: 2222}),
			ssh.WithCommand("sshfs -o allow_other -p 2222 root@localhost:/ " + targetPath + " && /bin/sleep infinity"),

			ssh.WithStderr(os.Stderr),
			ssh.WithStdout(os.Stdout),
		}

		c := ssh.New(addr, options...)

		go func() {
			if err := c.Run(ctx); err != nil {
				cli.Error(err)
			}

			cancel()
		}()
	}

	// if len(container.Volumes) > 0 && (options.SyncMode == SyncModeLocal || options.SyncMode == SyncModeRemote) {
	// 	config := &gossh.ClientConfig{
	// 		User: "root",

	// 		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	// 	}

	// 	conn, err := gossh.Dial("tcp", addr, config)

	// 	if err != nil {
	// 		return err
	// 	}

	// 	local, err := osfs.NewWatchableFS()

	// 	if err != nil {
	// 		return err
	// 	}

	// 	remote, err := sftpfs.NewWatchableFS(conn)

	// 	if err != nil {
	// 		return err
	// 	}

	// 	if options.SyncMode == SyncModeLocal {
	// 		go func() {
	// 			if err := syncLocalChanges(ctx, local, remote, container.Volumes); err != nil {
	// 				cli.Error(err)
	// 			}

	// 			cancel()
	// 		}()
	// 	}

	// 	if options.SyncMode == SyncModeRemote {
	// 		go func() {
	// 			if err := syncRemoteVolumes(ctx, local, remote, container.Volumes); err != nil {
	// 				cli.Error(err)
	// 			}

	// 			cancel()
	// 		}()
	// 	}
	// }

	<-ctx.Done()

	return nil
}

func tunnelPorts(ctx context.Context, addr string, ports []Port) error {
	var options []ssh.Option

	for _, p := range ports {
		options = append(options, ssh.WithLocalPortForward(ssh.PortForward{LocalPort: p.Source, RemotePort: p.Target}))
	}

	client := ssh.New(addr, options...)

	return client.Run(ctx)
}

func copyVolumes(ctx context.Context, client kubernetes.Client, namespace, name string, volumes []Volume) error {
	for _, v := range volumes {
		path := path.Join("/data", v.Target)

		options := &archive.TarOptions{}

		if v.Identity != nil {
			options.ChownOpts = &idtools.Identity{
				UID: v.Identity.UID,
				GID: v.Identity.GID,
			}
		}

		tar, err := archive.TarWithOptions(v.Source, options)

		if err != nil {
			return err
		}

		if err := client.PodExec(ctx, namespace, name, "loop-tunnel", []string{"tar", "xf", "-", "-C", path}, false, tar, os.Stdout, os.Stdout); err != nil {
			return err
		}
	}

	return nil
}
