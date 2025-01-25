package docker

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"

	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var Command = &cli.Command{
	Name:  "docker",
	Usage: "use cluster Docker daemon",

	Flags: []cli.Flag{
		app.NamespaceFlag,
	},

	Action: func(ctx context.Context, cmd *cli.Command) error {
		client := app.MustClient(ctx, cmd)

		port := app.MustRandomPort(ctx, cmd, 2375)
		namespace := app.Namespace(ctx, cmd)

		if namespace == "" {
			namespace = client.Namespace()
		}

		return connectDaemon(ctx, client, namespace, port)
	},
}

func connectDaemon(ctx context.Context, client kubernetes.Client, namespace string, port int) error {
	if namespace == "" {
		namespace = client.Namespace()
	}

	name := "loop-docker-" + uuid.New().String()[0:7]

	docker := "docker"

	loopContext := "loop"
	currentContext := "default"

	if val, err := exec.Command(docker, "context", "show").Output(); err == nil {
		currentContext = strings.TrimRight(string(val), "\n")
	}

	defer func() {
		cli.Info("★ resetting Docker context to \"" + currentContext + "\"")
		exec.Command(docker, "context", "use", currentContext).Run()
		exec.Command(docker, "context", "rm", loopContext).Run()
	}()

	defer func() {
		cli.Infof("★ removing container (%s/%s)...", namespace, name)
		deleteDaemon(context.Background(), client, namespace, name)
	}()

	cli.Infof("★ creating container (%s/%s)...", namespace, name)
	pod, err := createDaemon(ctx, client, namespace, name)

	if err != nil {
		return err
	}

	ports := map[int]int{
		port: 2375,
	}

	ready := make(chan struct{})

	go func() {
		<-ready

		cli.Info("★ setting Docker context to \"" + loopContext + "\"")
		exec.Command(docker, "context", "rm", loopContext).Run()
		exec.Command(docker, "context", "create", loopContext, "--docker", fmt.Sprintf("host=tcp://127.0.0.1:%d", port)).Run()
		exec.Command(docker, "context", "use", loopContext).Run()

		cli.Info("Press ctrl-c to stop Docker daemon")
	}()

	if err := client.PodPortForward(ctx, pod.Namespace, pod.Name, "", ports, ready); err != nil {
		return err
	}

	return nil
}

func createDaemon(ctx context.Context, client kubernetes.Client, namespace, name string) (*corev1.Pod, error) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},

		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "docker",

					Image:           "public.ecr.aws/docker/library/docker:27-dind-rootless",
					ImagePullPolicy: corev1.PullAlways,

					SecurityContext: &corev1.SecurityContext{
						Privileged: kubernetes.Ptr(true),
					},

					Env: []corev1.EnvVar{
						{
							Name:  "DOCKER_TLS_CERTDIR",
							Value: "",
						},
					},

					Args: []string{
						"--tls=false",
					},

					Ports: []corev1.ContainerPort{
						{
							Name:          "docker",
							Protocol:      corev1.ProtocolTCP,
							ContainerPort: int32(2375),
						},
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

			TerminationGracePeriodSeconds: kubernetes.Ptr(int64(10)),
		},
	}

	if _, err := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return nil, err
	}

	pod, err := client.WaitForPod(ctx, namespace, name)
	time.Sleep(10 * time.Second)

	return pod, err
}

func deleteDaemon(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	if err := client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		//return err
	}

	return nil
}
