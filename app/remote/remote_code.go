package remote

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/system"
	"github.com/adrianliechti/loop/pkg/to"
	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var codeCommand = &cli.Command{
	Name:  "code",
	Usage: "run cluster VSCode Server",

	Flags: []cli.Flag{
		app.NamespaceFlag,
		app.KubeconfigFlag,
		&cli.StringFlag{
			Name:  app.PortFlag.Name,
			Usage: "local server port",
		},
		&cli.StringFlag{
			Name:  "stack",
			Usage: "language stack",
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

		stacks := []string{
			"dotnet",
			"golang",
			"java",
		}

		stack := c.String("stack")

		if stack == "" {
			i, _, err := cli.Select("select stack", stacks)

			if err != nil {
				return err
			}

			stack = stacks[i]
		}

		stack = strings.ToLower(stack)

		port := app.MustPortOrRandom(c, 3000)
		namespace := app.Namespace(c)

		if namespace == "" {
			namespace = client.Namespace()
		}

		return runCode(c.Context, client, stack, port, namespace, path, nil)
	},
}

func runCode(ctx context.Context, client kubernetes.Client, stack string, port int, namespace, path string, ports map[int]int) error {
	if namespace == "" {
		namespace = client.Namespace()
	}

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

	cli.Infof("Starting remote VSCode...")
	pod, err := startCodeContainer(ctx, client, namespace, "adrianliechti/loop-code:"+stack)

	if err != nil {
		return err
	}

	defer func() {
		cli.Infof("Stopping remote VSCode (%s/%s)...", namespace, pod)
		stopCodeContainer(context.Background(), client, namespace, pod)
	}()

	time.AfterFunc(5*time.Second, func() {
		cli.OpenURL(fmt.Sprintf("http://localhost:%d", port))
	})

	tunnelPorts := map[int]int{
		port: 3000,
	}

	cli.Info("Press ctrl-c to stop remote VSCode server")

	return runTunnel(ctx, client, namespace, pod, containerPort, tunnelPorts)
}

func startCodeContainer(ctx context.Context, client kubernetes.Client, namespace, image string) (string, error) {
	name := "loop-code-" + uuid.New().String()[0:7]

	mountPropagationBidirectional := corev1.MountPropagationBidirectional
	mountPropagationHostToContainer := corev1.MountPropagationHostToContainer

	if _, err := client.CoreV1().ServiceAccounts(namespace).Create(ctx, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}, metav1.CreateOptions{}); err != nil {
		return "", err
	}

	if _, err := client.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      name,
				Namespace: namespace,
			},
		},
	}, metav1.CreateOptions{}); err != nil {
		return "", err
	}

	if _, err := client.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},

		Spec: corev1.PodSpec{
			ServiceAccountName: name,

			InitContainers: []corev1.Container{
				{
					Name: "init-workspace",

					Image:           "busybox:stable",
					ImagePullPolicy: corev1.PullAlways,

					Command: []string{
						"chown",
						"1000:1000",
						"/mnt",
					},

					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "mnt",
							MountPath: "/mnt",

							MountPropagation: &mountPropagationHostToContainer,
						},
					},
				},
			},

			Containers: []corev1.Container{
				{
					Name: "code",

					Image:           image,
					ImagePullPolicy: corev1.PullAlways,

					SecurityContext: &corev1.SecurityContext{
						RunAsUser: to.Int64Ptr(1000),
					},

					Env: []corev1.EnvVar{
						{
							Name:  "DOCKER_HOST",
							Value: "tcp://127.0.0.1:2375",
						},
					},

					Ports: []corev1.ContainerPort{
						{
							Name:          "http",
							Protocol:      corev1.ProtocolTCP,
							ContainerPort: int32(3000),
						},
					},

					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "mnt",
							MountPath: "/mnt",

							MountPropagation: &mountPropagationHostToContainer,
						},
					},
				},
				{
					Name: "docker",

					Image:           "docker:24-dind-rootless",
					ImagePullPolicy: corev1.PullAlways,

					SecurityContext: &corev1.SecurityContext{
						Privileged: to.BoolPtr(true),
					},

					Env: []corev1.EnvVar{
						{
							Name:  "DOCKER_TLS_CERTDIR",
							Value: "",
						},
					},

					Args: []string{
						"--group",
						"1000",
						"--tls=false",
					},

					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "mnt",
							MountPath: "/mnt",

							MountPropagation: &mountPropagationHostToContainer,
						},

						{
							Name:      "docker",
							MountPath: "/var/lib/docker",
						},
						{
							Name:      "modules",
							MountPath: "/lib/modules",
							ReadOnly:  true,
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
				{
					Name: "docker",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: "modules",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/lib/modules",
						},
					},
				},
			},

			TerminationGracePeriodSeconds: to.Int64Ptr(10),
		},
	}, metav1.CreateOptions{}); err != nil {
		return "", err
	}

	if _, err := client.WaitForPod(ctx, namespace, name); err != nil {
		return "", err
	}

	return name, nil
}

func stopCodeContainer(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	if err := client.CoreV1().ServiceAccounts(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		// return err
	}

	if err := client.RbacV1().ClusterRoleBindings().Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		// return err
	}

	return client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: to.Int64Ptr(0),
	})
}
