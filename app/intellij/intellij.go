package intellij

import (
	"context"
	"fmt"
	"time"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/to"

	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var Command = &cli.Command{
	Name:  "intellij",
	Usage: "start cluster IntelliJ instance",

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

	name := "loop-intellij-" + uuid.New().String()[0:7]

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
		port: 22,
	}

	ready := make(chan struct{})

	go func() {
		<-ready

		cli.Info(fmt.Sprintf("host=tcp://127.0.0.1:%d", port))

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
					Name: "intellij",

					Image:           "ghcr.io/adrianliechti/loop-tunnel:latest",
					ImagePullPolicy: corev1.PullAlways,
				},
			},

			TerminationGracePeriodSeconds: to.Ptr(int64(10)),
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
