package code

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/remote/run"
	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
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

	Action: func(ctx context.Context, cmd *cli.Command) error {
		client := app.MustClient(ctx, cmd)

		path, err := os.Getwd()

		if err != nil {
			return err
		}

		stacks := []string{
			"default",
			"golang",
			"python",
			"java",
			"dotnet",
		}

		stack := cmd.String("stack")

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

		image := "ghcr.io/adrianliechti/loop-code"

		if stack != "" {
			image += ":" + strings.ToLower(stack)
		}

		port := app.MustPortOrRandom(ctx, cmd, 8888)
		namespace := app.Namespace(ctx, cmd)

		if namespace == "" {
			namespace = client.Namespace()
		}

		ports, _ := app.Ports(ctx, cmd)

		if ports == nil {
			ports = map[int]int{}
		}

		ports[port] = 3000

		var runPorts []run.Port

		for s, t := range ports {
			runPorts = append(runPorts, run.Port{
				Source: s,
				Target: t,
			})
		}

		var runVolumes []run.Volume

		runVolumes = append(runVolumes, run.Volume{
			Source: path,
			Target: "/src",

			Identity: &run.Identity{
				UID: 1000,
				GID: 1000,
			},
		})

		container := &run.Container{
			Image: image,

			Stdout: os.Stdout,
			Stderr: os.Stderr,

			Ports:   runPorts,
			Volumes: runVolumes,
		}

		options := &run.RunOptions{
			Name:      "loop-code-" + uuid.NewString()[0:7],
			Namespace: namespace,

			SyncMode: run.SyncModeMount,
		}

		options.OnReady = func(ctx context.Context, client kubernetes.Client, pod *corev1.Pod) error {
			cli.OpenURL(fmt.Sprintf("http://localhost:%d", port))

			return nil
		}

		return run.Run(ctx, client, container, options)
	},
}

// func Run(ctx context.Context, client kubernetes.Client, stack string, port int, namespace, path string, ports map[int]int) error {
// 	if namespace == "" {
// 		namespace = client.Namespace()
// 	}

// 	image := "ghcr.io/adrianliechti/loop-code"

// 	if stack != "" {
// 		image += ":" + strings.ToLower(stack)
// 	}

// 	name := "loop-code-" + uuid.New().String()[0:7]

// 	cli.Infof("★ creating container (%s/%s)...", namespace, name)
// 	pod, err := startPod(ctx, client, namespace, name, image)

// 	if err != nil {
// 		return err
// 	}

// 	defer func() {
// 		cli.Infof("★ removing container (%s/%s)...", pod.Namespace, pod.Name)
// 		stopPod(context.Background(), client, pod.Namespace, pod.Name)
// 	}()

// 	time.AfterFunc(5*time.Second, func() {
// 		cli.OpenURL(fmt.Sprintf("http://localhost:%d", port))
// 	})

// 	if ports == nil {
// 		ports = map[int]int{}
// 	}

// 	ports[port] = 3000

// 	cli.Info("Press ctrl-c to stop remote VSCode server")

// 	return remote.Run(ctx, client, pod.Namespace, pod.Name, path, ports)
// }

// func startPod(ctx context.Context, client kubernetes.Client, namespace, name, image string) (*corev1.Pod, error) {
// 	serviceaccount := &corev1.ServiceAccount{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name: name,
// 		},
// 	}

// 	if _, err := client.CoreV1().ServiceAccounts(namespace).Create(ctx, serviceaccount, metav1.CreateOptions{}); err != nil {
// 		return nil, err
// 	}

// 	clusterrolebinding := &rbacv1.ClusterRoleBinding{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name: name,
// 		},

// 		RoleRef: rbacv1.RoleRef{
// 			APIGroup: "rbac.authorization.k8s.io",
// 			Kind:     "ClusterRole",
// 			Name:     "cluster-admin",
// 		},

// 		Subjects: []rbacv1.Subject{
// 			{
// 				Kind:      "ServiceAccount",
// 				Name:      name,
// 				Namespace: namespace,
// 			},
// 		},
// 	}

// 	if _, err := client.RbacV1().ClusterRoleBindings().Create(ctx, clusterrolebinding, metav1.CreateOptions{}); err != nil {
// 		return nil, err
// 	}

// 	pod := &corev1.Pod{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name: name,
// 		},

// 		Spec: corev1.PodSpec{
// 			ServiceAccountName: name,

// 			InitContainers: []corev1.Container{
// 				{
// 					Name: "init-workspace",

// 					Image:           "public.ecr.aws/docker/library/busybox:stable",
// 					ImagePullPolicy: corev1.PullAlways,

// 					Command: []string{
// 						"chown",
// 						"1000:1000",
// 						"/mnt",
// 					},
// 				},
// 			},

// 			Containers: []corev1.Container{
// 				{
// 					Name: "code",

// 					Image:           image,
// 					ImagePullPolicy: corev1.PullAlways,

// 					SecurityContext: &corev1.SecurityContext{
// 						RunAsUser: to.Ptr(int64(1000)),
// 					},

// 					Env: []corev1.EnvVar{
// 						{
// 							Name:  "DOCKER_HOST",
// 							Value: "tcp://127.0.0.1:2375",
// 						},
// 					},

// 					Ports: []corev1.ContainerPort{
// 						{
// 							Name:          "http",
// 							Protocol:      corev1.ProtocolTCP,
// 							ContainerPort: int32(3000),
// 						},
// 					},
// 				},
// 				{
// 					Name: "docker",

// 					Image:           "public.ecr.aws/docker/library/docker:27-dind-rootless",
// 					ImagePullPolicy: corev1.PullAlways,

// 					SecurityContext: &corev1.SecurityContext{
// 						Privileged: to.Ptr(true),
// 					},

// 					Env: []corev1.EnvVar{
// 						{
// 							Name:  "DOCKER_TLS_CERTDIR",
// 							Value: "",
// 						},
// 					},

// 					Args: []string{
// 						"--group",
// 						"1000",
// 						"--tls=false",
// 					},

// 					VolumeMounts: []corev1.VolumeMount{
// 						{
// 							Name:      "docker",
// 							MountPath: "/var/lib/docker",
// 						},
// 						{
// 							Name:      "modules",
// 							MountPath: "/lib/modules",
// 							ReadOnly:  true,
// 						},
// 					},
// 				},
// 			},

// 			Volumes: []corev1.Volume{
// 				{
// 					Name: "docker",
// 					VolumeSource: corev1.VolumeSource{
// 						EmptyDir: &corev1.EmptyDirVolumeSource{},
// 					},
// 				},
// 				{
// 					Name: "modules",
// 					VolumeSource: corev1.VolumeSource{
// 						HostPath: &corev1.HostPathVolumeSource{
// 							Path: "/lib/modules",
// 						},
// 					},
// 				},
// 			},

// 			TerminationGracePeriodSeconds: to.Ptr(int64(10)),
// 		},
// 	}

// 	if err := remote.UpdatePod(pod, "/mnt"); err != nil {
// 		return nil, err
// 	}

// 	if _, err := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
// 		return nil, err
// 	}

// 	return client.WaitForPod(ctx, namespace, name)
// }

// func stopPod(ctx context.Context, client kubernetes.Client, namespace, name string) error {
// 	if err := client.CoreV1().ServiceAccounts(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
// 		// return err
// 	}

// 	if err := client.RbacV1().ClusterRoleBindings().Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
// 		// return err
// 	}

// 	return client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
// 		GracePeriodSeconds: to.Ptr(int64(0)),
// 	})
// }
