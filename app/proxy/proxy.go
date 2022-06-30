package proxy

import (
	"context"
	"time"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/to"

	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var Command = &cli.Command{
	Name:  "proxy",
	Usage: "Proxy Kubernetes network (SOCKS5)",

	Flags: []cli.Flag{
		app.NamespaceFlag,
		app.KubeconfigFlag,
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		port := app.MustRandomPort(c, 1080)
		namespace := app.Namespace(c)

		if namespace == nil {
			namespace = to.StringPtr(client.Namespace())
		}

		return runProxy(c.Context, client, *namespace, port)
	},
}

func runProxy(ctx context.Context, client kubernetes.Client, namespace string, port int) error {
	name := "loop-proxy-" + uuid.New().String()[0:7]

	defer func() {
		cli.Infof("Stopping proxy pod (%s/%s)...", namespace, name)
		deleteProxy(context.Background(), client, namespace, name)
	}()

	cli.Infof("Starting proxy pod (%s/%s)...", namespace, name)
	pod, err := createProxy(ctx, client, namespace, name)

	if err != nil {
		return err
	}

	ports := map[int]int{
		port: 1080,
	}

	ready := make(chan struct{})

	go func() {
		<-ready

		cli.Infof("export http_proxy=socks5h://localhost:%d", port)
		cli.Infof("export https_proxy=socks5h://localhost:%d", port)

		cli.Info("Press ctrl-c to stop SOCKS proxy")
	}()

	if err := client.PodPortForward(ctx, pod.Namespace, pod.Name, "", ports, ready); err != nil {
		return err
	}

	return nil
}

func createProxy(ctx context.Context, client kubernetes.Client, namespace, name string) (*corev1.Pod, error) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app.kubernetes.io/name":     "loop-proxy",
				"app.kubernetes.io/instance": name,
			},
		},

		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "proxy",
					Image: "adrianliechti/loop-socks:0",

					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
				},
			},
		},
	}

	if _, err := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return nil, err
	}

	pod, err := client.WaitForPod(ctx, namespace, name)
	time.Sleep(10 * time.Second)

	return pod, err
}

func deleteProxy(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: to.Int64Ptr(0),
	})

	return nil
}
