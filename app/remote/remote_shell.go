package remote

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
	"github.com/adrianliechti/loop/pkg/kubectl"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/ssh"
	"github.com/adrianliechti/loop/pkg/system"
	"github.com/adrianliechti/loop/pkg/to"
	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var shellCommand = &cli.Command{
	Name:  "shell",
	Usage: "run cluster shell",

	Flags: []cli.Flag{
		app.NamespaceFlag,
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

		path, err := os.Getwd()

		if err != nil {
			return err
		}

		image := c.String("image")
		namespace := app.Namespace(c)

		if namespace == nil {
			namespace = to.StringPtr(client.Namespace())
		}

		if image == "" {
			image = "debian"
		}

		return runShell(c.Context, client, *namespace, image, true, true, path, nil)
	},
}

func runShell(ctx context.Context, client kubernetes.Client, namespace, image string, stdin, tty bool, path string, ports map[int]int) error {
	containerPort, err := system.FreePort(0)

	if err != nil {
		return err
	}

	cli.Infof("Starting helper container...")
	container, err := startServer(ctx, path, containerPort)

	if err != nil {
		return err
	}

	defer func() {
		cli.Infof("Stopping helper container (%s)...", container)
		stopServer(context.Background(), container)
	}()

	cli.Infof("Starting remote container...")
	pod, err := startPod(ctx, client, namespace, image, stdin, tty)

	if err != nil {
		return err
	}

	defer func() {
		cli.Infof("Stopping remote container (%s/%s)...", namespace, pod)
		stopPod(context.Background(), client, namespace, pod)
	}()

	go func() {
		if err := runTunnel(ctx, client, namespace, pod, containerPort, ports); err != nil {
			cli.Error(err)
		}
	}()

	return kubectl.Attach(ctx, client.ConfigPath(), namespace, pod, "shell")
}

func startServer(ctx context.Context, path string, port int) (string, error) {
	tool, _, err := docker.Tool(ctx)

	if err != nil {
		return "", err
	}

	args := []string{
		"run",
		"-d",

		"--pull",
		"always",

		"--publish",
		fmt.Sprintf("127.0.0.1:%d:22", port),

		"--volume",
		path + ":/src",

		"adrianliechti/loop-tunnel",
	}

	cmd := exec.CommandContext(ctx, tool, args...)

	output, err := cmd.CombinedOutput()

	if err != nil {
		return "", errors.New(string(output))
	}

	text := string(output)
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.TrimRight(text, "\n")

	lines := strings.Split(text, "\n")

	if len(lines) == 0 {
		return "", errors.New("unable to get container id")
	}

	container := lines[len(lines)-1]
	return container, nil
}

func stopServer(ctx context.Context, container string) error {
	tool, _, err := docker.Tool(ctx)

	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, tool, "rm", "--force", container)

	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.New(string(output))
	}

	return err
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
		GracePeriodSeconds: to.Int64Ptr(0),
	})

	return err
}

func runTunnel(ctx context.Context, client kubernetes.Client, namespace, name string, port int, tunnels map[int]int) error {
	ssh, _, err := ssh.Tool(ctx)

	if err != nil {
		return err
	}

	kubectl, _, err := kubectl.Tool(ctx)

	if err != nil {
		return err
	}

	args := []string{
		"-q",
		"-t",
		"-l",
		"root",
		"-o",
		"UserKnownHostsFile=/dev/null",
		"-o",
		"StrictHostKeyChecking=no",
		"-o",
		fmt.Sprintf("ProxyCommand=%s --kubeconfig %s exec -i -n %s %s -c ssh -- nc 127.0.0.1 22", kubectl, client.ConfigPath(), namespace, name),
		"localhost",
	}

	command := "mkdir -p /mnt/src && sshfs -o allow_other -p 2222 root@localhost:/src /mnt/src && exec /bin/ash"

	if port != 0 {
		args = append(args, "-R", fmt.Sprintf("2222:127.0.0.1:%d", port))
	}

	for source, target := range tunnels {
		args = append(args, "-L", fmt.Sprintf("%d:127.0.0.1:%d", source, target))
	}

	if command != "" {
		args = append(args, command)
	}

	cmd := exec.CommandContext(ctx, ssh, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return cmd.Run()
}
