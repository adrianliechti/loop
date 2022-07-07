package application

import (
	"context"
	"io"
	"os"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/kubernetes/resource"
	"github.com/adrianliechti/loop/pkg/to"

	corev1 "k8s.io/api/core/v1"
)

var logCommand = &cli.Command{
	Name:  "logs",
	Usage: "stream application logs",

	Flags: []cli.Flag{
		app.NameFlag,
		app.NamespaceFlag,
		app.KubeconfigFlag,
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		name := app.Name(c)
		namespace := app.Namespace(c)

		if name != nil && namespace == nil {
			namespace = to.StringPtr(client.Namespace())
		}

		if name == nil {
			app := MustApplication(c.Context, client, to.String(namespace))

			name = &app.Name
			namespace = &app.Namespace
		}

		return applicationLogs(c.Context, client, *namespace, *name)
	},
}

func applicationLogs(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	app, err := resource.App(ctx, client, namespace, name)

	if err != nil {
		return err
	}

	for _, r := range app.Resources {
		if pod, ok := r.Object.(corev1.Pod); ok {
			for _, container := range pod.Spec.Containers {
				streamLogs(ctx, client, pod.Namespace, pod.Name, container.Name)
			}
		}
	}

	return nil
}

func streamLogs(ctx context.Context, client kubernetes.Client, namespace, name, container string) error {
	req := client.CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{
		Follow:    true,
		Container: container,
	})

	reader, err := req.Stream(ctx)

	if err != nil {
		cli.Error(err)
		return nil
	}

	defer reader.Close()

	if _, err := io.Copy(os.Stdout, reader); err != nil {
		return err
	}

	return nil
}
