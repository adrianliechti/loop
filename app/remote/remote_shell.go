package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubectl"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/sftp"
	sshtool "github.com/adrianliechti/loop/pkg/ssh"
	"github.com/adrianliechti/loop/pkg/system"
	"github.com/adrianliechti/loop/pkg/to"
	"github.com/google/uuid"

	"github.com/gliderlabs/ssh"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var shellCommand = &cli.Command{
	Name:  "shell",
	Usage: "run cluster shell",

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

		if namespace == "" {
			namespace = client.Namespace()
		}

		if image == "" {
			image = "debian"
		}

		return runShell(c.Context, client, namespace, image, true, true, path, nil)
	},
}

func runShell(ctx context.Context, client kubernetes.Client, namespace, image string, stdin, tty bool, path string, ports map[int]int) error {
	sshdPort, err := system.FreePort(0)

	if err != nil {
		return err
	}

	cli.Infof("Starting ssh server...")
	if err := startServer(ctx, sshdPort, path, ports); err != nil {
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
		if err := runTunnel(ctx, client, namespace, pod, path, sshdPort, ports); err != nil {
			cli.Error(err)
		}
	}()

	return kubectl.Attach(ctx, client.ConfigPath(), namespace, pod, "shell")
}

func startServer(ctx context.Context, port int, path string, ports map[int]int) error {
	forwardHandler := &ssh.ForwardedTCPHandler{}

	s := ssh.Server{
		Addr: fmt.Sprintf("127.0.0.1:%d", port),

		Handler: func(s ssh.Session) {
			io.WriteString(s, "SSH server operational. Use SFTP for file transfer.\n")
		},

		RequestHandlers: map[string]ssh.RequestHandler{
			"tcpip-forward":        forwardHandler.HandleSSHRequest,
			"cancel-tcpip-forward": forwardHandler.HandleSSHRequest,
		},

		LocalPortForwardingCallback: ssh.LocalPortForwardingCallback(func(ctx ssh.Context, host string, port uint32) bool {
			return false
		}),

		ReversePortForwardingCallback: ssh.ReversePortForwardingCallback(func(ctx ssh.Context, host string, port uint32) bool {
			return false
		}),

		PublicKeyHandler: ssh.PublicKeyHandler(func(ctx ssh.Context, key ssh.PublicKey) bool {
			return true
		}),

		PasswordHandler: ssh.PasswordHandler(func(ctx ssh.Context, password string) bool {
			return true
		}),

		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": func(s ssh.Session) {

				srv := sftp.New(s, path)

				if err := srv.Serve(); err != nil {
					srv.Close()
				}
			},
		},
	}

	go func() {
		<-ctx.Done()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := s.Shutdown(ctx); err != nil {
			if errors.Is(err, ssh.ErrServerClosed) {
				return
			}

			log.Println("could not stop server", "error", err)
		}

		if err := s.Close(); err != nil {
			log.Println("could not close server", "error", err)
		}
	}()

	go func() {
		if err := s.ListenAndServe(); err != nil {
			if errors.Is(err, ssh.ErrServerClosed) {
				return
			}

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

func runTunnel(ctx context.Context, client kubernetes.Client, namespace, name, path string, port int, tunnels map[int]int) error {
	ssh, _, err := sshtool.Info(ctx)

	if err != nil {
		return err
	}

	kubectl, _, err := kubectl.Info(ctx)

	if err != nil {
		return err
	}

	args := []string{
		"-q",
		"-t",
		"-l", "root",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "StrictHostKeyChecking=no",
		"-o", fmt.Sprintf("ProxyCommand=%s --kubeconfig %s exec -i -n %s %s -c ssh -- nc 127.0.0.1 22", kubectl, client.ConfigPath(), namespace, name),
		"localhost",
	}

	command := "mkdir -p /mnt/src && sshfs -o allow_other -p 2222 root@localhost:/ /mnt/src && exec /bin/sh"

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
