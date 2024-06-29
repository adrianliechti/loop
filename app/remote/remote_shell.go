package remote

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/sftp"
	"github.com/adrianliechti/loop/pkg/ssh"
	"github.com/adrianliechti/loop/pkg/system"
	"github.com/adrianliechti/loop/pkg/to"

	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var shellCommand = &cli.Command{
	Name:  "shell",
	Usage: "run cluster Shell",

	Flags: []cli.Flag{
		app.NamespaceFlag,
		app.KubeconfigFlag,

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

		if image == "" {
			image = "debian"
		}

		return runShell(c.Context, client, namespace, image, true, true, path, nil)
	},
}

func runShell(ctx context.Context, client kubernetes.Client, namespace, image string, stdin, tty bool, path string, ports map[int]int) error {
	if namespace == "" {
		namespace = client.Namespace()
	}

	sftpport, err := system.FreePort(0)

	if err != nil {
		return err
	}

	cli.Infof("Starting sftp server...")
	if err := startServer(ctx, sftpport, path); err != nil {
		return err
	}

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
		if err := runTunnel(ctx, client, namespace, pod, sftpport, ports); err != nil {
			cli.Error(err)
		}
	}()

	return client.PodAttach(ctx, namespace, pod, "shell", tty, os.Stdin, os.Stdout, os.Stderr)
}

func startServer(ctx context.Context, port int, path string) error {
	s := sftp.NewServer(fmt.Sprintf("127.0.0.1:%d", port), path)

	go func() {
		<-ctx.Done()
		s.Close()
	}()

	go func() {
		if err := s.ListenAndServe(); err != nil {
			log.Println("could not start server", "error", err)
		}
	}()

	return nil
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
						Privileged: to.Ptr(true),
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

			TerminationGracePeriodSeconds: to.Ptr(int64(10)),
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
		GracePeriodSeconds: to.Ptr(int64(0)),
	})

	return err
}

func runTunnel(ctx context.Context, client kubernetes.Client, namespace, name string, port int, tunnels map[int]int) error {
	localport, err := system.FreePort(0)

	if err != nil {
		return err
	}

	options := []ssh.Option{
		ssh.WithCommand("mkdir -p /mnt/src && sshfs -o allow_other -p 2222 root@localhost:/ /mnt/src && /bin/sleep infinity"),
	}

	if port > 0 {
		options = append(options, ssh.WithRemotePortForward(ssh.PortForward{LocalPort: port, RemotePort: 2222}))
	}

	for s, t := range tunnels {
		options = append(options, ssh.WithLocalPortForward(ssh.PortForward{LocalPort: s, RemotePort: t}))
	}

	c := ssh.New(fmt.Sprintf("127.0.0.1:%d", localport), options...)

	ready := make(chan struct{})

	go func() {
		<-ready

		if err := c.Run(ctx); err != nil {
			cli.Error(err)
		}
	}()

	if err := client.PodPortForward(ctx, namespace, name, "", map[int]int{localport: 22}, ready); err != nil {
		return err
	}

	return nil
}
