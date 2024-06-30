package code

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/remote"
	"github.com/adrianliechti/loop/pkg/to"
	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var Command = &cli.Command{
	Name:  "code",
	Usage: "run cluster VS Code",

	Flags: []cli.Flag{
		app.NamespaceFlag,

		&cli.StringFlag{
			Name:  "stack",
			Usage: "language stack",
		},

		app.PortsFlag,
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		path, err := os.Getwd()

		if err != nil {
			return err
		}

		stacks := []string{
			"default",
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

		if stack == "latest" || stack == "default" {
			stack = ""
		}

		port := app.MustPortOrRandom(c, 8888)
		namespace := app.Namespace(c)

		if namespace == "" {
			namespace = client.Namespace()
		}

		tunnels, _ := app.Ports(c)

		return Run(c.Context, client, stack, port, namespace, path, tunnels)
	},
}

func Run(ctx context.Context, client kubernetes.Client, stack string, port int, namespace, path string, ports map[int]int) error {
	if namespace == "" {
		namespace = client.Namespace()
	}

	image := "adrianliechti/loop-code"

	if stack != "" {
		image += ":" + strings.ToLower(stack)
	}

	name := "loop-code-" + uuid.New().String()[0:7]

	cli.Infof("Starting VSCode pod (%s/%s)...", namespace, name)
	pod, err := startPod(ctx, client, namespace, name, image)

	if err != nil {
		return err
	}

	defer func() {
		cli.Infof("Stopping VSCode pod (%s/%s)...", pod.Namespace, pod.Name)
		stopPod(context.Background(), client, pod.Namespace, pod.Name)
	}()

	time.AfterFunc(5*time.Second, func() {
		cli.OpenURL(fmt.Sprintf("http://localhost:%d", port))
	})

	if ports == nil {
		ports = map[int]int{}
	}

	ports[port] = 3000

	cli.Info("Press ctrl-c to stop remote VSCode server")

	return remote.Run(ctx, client, pod.Namespace, pod.Name, path, ports)
}

func startPod(ctx context.Context, client kubernetes.Client, namespace, name, image string) (*corev1.Pod, error) {
	serviceaccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	if _, err := client.CoreV1().ServiceAccounts(namespace).Create(ctx, serviceaccount, metav1.CreateOptions{}); err != nil {
		return nil, err
	}

	clusterrolebinding := &rbacv1.ClusterRoleBinding{
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
	}

	if _, err := client.RbacV1().ClusterRoleBindings().Create(ctx, clusterrolebinding, metav1.CreateOptions{}); err != nil {
		return nil, err
	}

	pod := &corev1.Pod{
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
				},
			},

			Containers: []corev1.Container{
				{
					Name: "code",

					Image:           image,
					ImagePullPolicy: corev1.PullAlways,

					SecurityContext: &corev1.SecurityContext{
						RunAsUser: to.Ptr(int64(1000)),
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
				},
				{
					Name: "docker",

					Image:           "docker:27-dind-rootless",
					ImagePullPolicy: corev1.PullAlways,

					SecurityContext: &corev1.SecurityContext{
						Privileged: to.Ptr(true),
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
			},

			Volumes: []corev1.Volume{
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

			TerminationGracePeriodSeconds: to.Ptr(int64(10)),
		},
	}

	if err := remote.UpdatePod(pod, "/mnt"); err != nil {
		return nil, err
	}

	if _, err := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return nil, err
	}

	return client.WaitForPod(ctx, namespace, name)
}

func stopPod(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	if err := client.CoreV1().ServiceAccounts(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		// return err
	}

	if err := client.RbacV1().ClusterRoleBindings().Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		// return err
	}

	return client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: to.Ptr(int64(0)),
	})
}
