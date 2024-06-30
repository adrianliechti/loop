package container

import (
	"context"
	"os"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/remote"
	"github.com/adrianliechti/loop/pkg/to"
	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var Command = &cli.Command{
	Name:  "container",
	Usage: "run cluster Container",

	Flags: []cli.Flag{
		app.NamespaceFlag,

		&cli.StringFlag{
			Name:  "image",
			Usage: "container image to start",
		},

		app.PortsFlag,
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		path, err := os.Getwd()

		if err != nil {
			return err
		}

		image := c.String("image")
		namespace := app.Namespace(c)

		if image == "" {
			image = "debian"
		}

		if namespace == "" {
			namespace = client.Namespace()
		}

		tunnels, _ := app.Ports(c)

		return RunShell(c.Context, client, namespace, image, true, true, path, tunnels)
	},
}

func RunShell(ctx context.Context, client kubernetes.Client, namespace, image string, stdin, tty bool, path string, ports map[int]int) error {
	if namespace == "" {
		namespace = client.Namespace()
	}

	name := "loop-shell-" + uuid.New().String()[0:7]

	cli.Infof("Starting pod (%s/%s)...", namespace, name)
	pod, err := startPod(ctx, client, namespace, name, image, stdin, tty)

	if err != nil {
		return err
	}

	defer func() {
		cli.Infof("Stopping pod (%s/%s)...", namespace, pod)
		stopPod(context.Background(), client, namespace, pod)
	}()

	go func() {
		if err := remote.Run(ctx, client, namespace, pod, path, ports); err != nil {
			cli.Error(err)
		}
	}()

	return client.PodAttach(ctx, namespace, pod, "", tty, os.Stdin, os.Stdout, os.Stderr)
}

func startPod(ctx context.Context, client kubernetes.Client, namespace, name, image string, stdin, tty bool) (string, error) {
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
				},
			},

			TerminationGracePeriodSeconds: to.Ptr(int64(10)),
		},
	}

	if err := remote.UpdatePod(pod, "/mnt"); err != nil {
		return "", err
	}

	pod, err := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})

	if err != nil {
		return "", err
	}

	if _, err := client.WaitForPod(ctx, pod.Namespace, pod.Name); err != nil {
		return "", err
	}

	// reader, err := os.Open(client.ConfigPath())

	// if err != nil {
	// 	return "", err
	// }

	// if err := client.CreateFileInPod(ctx, pod.Namespace, pod.Name, "shell", "/run/secrets/loop/kubeconfig", reader); err != nil {
	// 	return "", err
	// }

	return pod.Name, nil
}

func stopPod(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	err := client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: to.Ptr(int64(0)),
	})

	return err
}
