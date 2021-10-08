package remote

import (
	"context"
	"os"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubectl"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/to"
	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var shellCommand = &cli.Command{
	Name:  "shell",
	Usage: "run cluster shell",

	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "image",
			Usage: "container image to start",
		},

		// &cli.StringSliceFlag{
		// 	Name:  app.PortsFlag.Name,
		// 	Usage: "forwarded ports",
		// },
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		serverPort := app.MustRandomPort(c, 0)
		serverPath, err := os.Getwd()

		if err != nil {
			return err
		}

		image := c.String("image")

		if image == "" {
			image = "debian"
		}

		return runShell(c.Context, client, serverPath, serverPort, image, true, true, nil)
	},
}

func runShell(ctx context.Context, client kubernetes.Client, path string, port int, image string, stdin, tty bool, ports map[int]int) error {
	container, err := startServer(ctx, path, port)

	if err != nil {
		return err
	}

	defer stopServer(context.Background(), container)

	namespace := "default"

	pod, err := startPod(ctx, client, namespace, image, stdin, tty)

	if err != nil {
		return err
	}

	defer stopPod(context.Background(), client, namespace, pod)

	go func() {
		if err := runTunnel(ctx, client, namespace, pod, port, ports); err != nil {
			cli.Error(err)
		}
	}()

	return kubectl.Attach(ctx, client.ConfigPath(), namespace, pod, "shell")
}

func startPod(ctx context.Context, client kubernetes.Client, namespace, image string, stdin, tty bool) (string, error) {
	name := "loop-shell-" + uuid.New().String()[0:7]

	mountPath := "/mnt"
	mountPropagationBidirectional := corev1.MountPropagationBidirectional
	mountPropagationHostToContainer := corev1.MountPropagationHostToContainer

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},

		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "shell",

					Image:           image,
					ImagePullPolicy: corev1.PullAlways,

					Stdin: stdin,
					TTY:   tty,

					VolumeMounts: []corev1.VolumeMount{
						{
							Name:             "mnt",
							MountPath:        mountPath,
							MountPropagation: &mountPropagationHostToContainer,
						},
					},
				},
				{
					Name: "ssh",

					Image:           "adrianliechti/loop-tunnel",
					ImagePullPolicy: corev1.PullAlways,

					SecurityContext: &corev1.SecurityContext{
						Privileged: to.BoolPtr(true),
					},

					VolumeMounts: []corev1.VolumeMount{
						{
							Name:             "mnt",
							MountPath:        "/mnt",
							MountPropagation: &mountPropagationBidirectional,
						},
					},
				},
			},

			Volumes: []corev1.Volume{
				{
					Name: "mnt",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},

			TerminationGracePeriodSeconds: to.Int64Ptr(10),
		},
	}

	pod, err := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})

	if err != nil {
		return "", err
	}

	if _, err := client.WaitForPod(ctx, namespace, pod.Name); err != nil {
		return "", err
	}

	return pod.Name, nil
}

func stopPod(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	err := client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: to.Int64Ptr(0),
	})

	return err
}
