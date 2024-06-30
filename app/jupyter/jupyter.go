package jupyter

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var Command = &cli.Command{
	Name:  "jupyter",
	Usage: "run cluster Jupyter",

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
			"datascience",
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

	if stack == "" {
		stack = "minimal"

	}

	image := "quay.io/jupyter/" + strings.ToLower(stack) + "-notebook"

	name := "loop-jupyter-" + uuid.New().String()[0:7]

	cli.Infof("Starting Jupyter pod (%s/%s)...", namespace, name)
	pod, err := startPod(ctx, client, namespace, name, image)

	if err != nil {
		return err
	}

	defer func() {
		cli.Infof("Stopping Jupyter pod (%s/%s)...", pod.Namespace, pod.Name)
		stopPod(context.Background(), client, pod.Namespace, pod.Name)
	}()

	time.AfterFunc(5*time.Second, func() {
		cli.OpenURL(fmt.Sprintf("http://localhost:%d", port))
	})

	if ports == nil {
		ports = map[int]int{}
	}

	ports[port] = 8888

	cli.Info("Press ctrl-c to stop remote Jupyter server")

	return remote.Run(ctx, client, pod.Namespace, pod.Name, path, ports)
}

func startPod(ctx context.Context, client kubernetes.Client, namespace, name, image string) (*corev1.Pod, error) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},

		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name: "init-workspace",

					Image:           "busybox:stable",
					ImagePullPolicy: corev1.PullAlways,

					Command: []string{
						"chown",
						"1000:100",
						"/home/jovyan/work",
					},
				},
			},

			Containers: []corev1.Container{
				{
					Name: "jupyter",

					Image:           image,
					ImagePullPolicy: corev1.PullAlways,

					SecurityContext: &corev1.SecurityContext{
						RunAsUser: to.Ptr(int64(1000)),
					},

					Command: []string{
						"start-notebook.sh",
					},

					Args: []string{
						"--ip='*'",
						"--NotebookApp.token=''",
						"--NotebookApp.password=''",
					},

					Ports: []corev1.ContainerPort{
						{
							Name:          "http",
							Protocol:      corev1.ProtocolTCP,
							ContainerPort: int32(8888),
						},
					},
				},
			},

			TerminationGracePeriodSeconds: to.Ptr(int64(10)),
		},
	}

	if err := remote.UpdatePod(pod, "/home/jovyan/work"); err != nil {
		return nil, err
	}

	if _, err := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return nil, err
	}

	return client.WaitForPod(ctx, namespace, name)
}

func stopPod(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	return client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: to.Ptr(int64(0)),
	})
}
