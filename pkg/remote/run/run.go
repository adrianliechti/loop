package run

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/to"
	"github.com/docker/docker/pkg/idtools"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/archive"

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

type RunOptions struct {
	Name      string
	Namespace string

	// Loop Tunnel image overwrite
	Image string

	OnPod   func(ctx context.Context, client kubernetes.Client, pod *corev1.Pod) error
	OnReady func(ctx context.Context, client kubernetes.Client, pod *corev1.Pod) error
}

func Run(ctx context.Context, client kubernetes.Client, container *Container, options *RunOptions) error {
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

	pod := templatePod(ctx, client, container, options)

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

	cli.Infof("★ copying volumes data...")

	if err := copyVolumes(ctx, client, pod.Namespace, pod.Name, container.Volumes); err != nil {
		return err
	}

	if options.OnReady != nil {
		if err := options.OnReady(ctx, client, pod); err != nil {
			return err
		}
	}

	return client.PodAttach(ctx, pod.Namespace, pod.Name, "main", container.TTY, container.Stdin, container.Stdout, container.Stderr)
}

func templatePod(ctx context.Context, client kubernetes.Client, container *Container, options *RunOptions) *corev1.Pod {
	var mounts []corev1.VolumeMount

	for _, v := range container.Volumes {
		mount := corev1.VolumeMount{
			Name: "loop-data",

			MountPath: v.Target,
			SubPath:   strings.TrimLeft(v.Target, "/"),
		}

		mounts = append(mounts, mount)
	}

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

					VolumeMounts: mounts,
				},
				{
					Name: "loop-tunnel",

					Image:           options.Image,
					ImagePullPolicy: corev1.PullAlways,

					VolumeMounts: []corev1.VolumeMount{
						{
							Name: "loop-data",

							MountPath: "/data",
						},
					},
				},
			},

			Volumes: []corev1.Volume{
				{
					Name: "loop-data",

					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},

			TerminationGracePeriodSeconds: to.Ptr(int64(10)),
		},
	}

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

func copyVolumes(ctx context.Context, client kubernetes.Client, namespace, name string, volumes []Volume) error {
	for _, v := range volumes {
		path := filepath.Join("/data", v.Target)

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
