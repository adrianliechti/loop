package remote

import (
	"context"
	"fmt"
	"os"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/to"

	"github.com/google/uuid"
	"github.com/moby/buildkit/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var buildCommand = &cli.Command{
	Name:  "build",
	Usage: "run cluster buildx builds",

	Flags: []cli.Flag{
		app.NamespaceFlag,
		app.KubeconfigFlag,
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		path, err := os.Getwd()

		if err != nil {
			return err
		}

		namespace := app.Namespace(c)
		port := app.MustPortOrRandom(c, 1234)

		return runBuild(c.Context, client, namespace, port, path)
	},
}

func runBuild(ctx context.Context, client kubernetes.Client, namespace string, port int, path string) error {
	if namespace == "" {
		namespace = client.Namespace()
	}

	cli.Infof("Starting remote BuildKit...")
	pod, err := startBuildKitContainer(ctx, client, namespace)

	if err != nil {
		return err
	}

	defer func() {
		cli.Infof("Stopping remote BuildKit (%s/%s)...", namespace, pod)
		stopBuildKitContainer(context.Background(), client, namespace, pod)
	}()

	ports := map[int]int{
		port: 1234,
	}

	ready := make(chan struct{})

	go func() {
		<-ready

		cli.Info("BuildKit ready")

		if err := buildImage(ctx, port); err != nil {
			cli.Error(err)
		}
	}()

	if err := client.PodPortForward(ctx, namespace, pod, "", ports, ready); err != nil {
		return err
	}

	return nil
}

func buildImage(ctx context.Context, port int) error {
	println("dial", port)

	client, err := client.New(ctx, fmt.Sprintf("tcp://127.0.0.1:%d", port))

	if err != nil {
		return err
	}

	println("du")

	du, err := client.DiskUsage(ctx)

	if err != nil {
		return err
	}

	for _, usage := range du {
		println(usage.ID, usage.Size)
	}

	return nil

}

func startBuildKitContainer(ctx context.Context, client kubernetes.Client, namespace string) (string, error) {
	name := "loop-buildkit-" + uuid.New().String()[0:7]

	if _, err := client.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},

		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "buildkit",

					Image:           "moby/buildkit:master",
					ImagePullPolicy: corev1.PullAlways,

					SecurityContext: &corev1.SecurityContext{
						Privileged: to.Ptr(true),

						// RunAsUser:  to.Ptr(int64(1000)),
						// RunAsGroup: to.Ptr(int64(1000)),

						// SeccompProfile: &corev1.SeccompProfile{
						// 	Type: corev1.SeccompProfileTypeUnconfined,
						// },
					},

					Args: []string{
						//"--oci-worker-no-process-sandbox",
						"--addr", "unix:///run/buildkit/buildkitd.sock",
						"--addr", "tcp://0.0.0.0:1234",
					},

					LivenessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{
									"buildctl",
									"debug",
									"workers",
								},
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       30,
					},

					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{
									"buildctl",
									"debug",
									"workers",
								},
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       30,
					},
				},
			},

			TerminationGracePeriodSeconds: to.Ptr(int64(10)),
		},
	}, metav1.CreateOptions{}); err != nil {
		return "", err
	}

	if _, err := client.WaitForPod(ctx, namespace, name); err != nil {
		return "", err
	}

	return name, nil
}

func stopBuildKitContainer(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	return client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: to.Ptr(int64(0)),
	})
}
