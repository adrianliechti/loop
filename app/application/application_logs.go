package application

import (
	"context"
	"os"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/kubernetes/resource"

	corev1 "k8s.io/api/core/v1"
)

var logCommand = &cli.Command{
	Name:  "logs",
	Usage: "stream application logs",

	Flags: []cli.Flag{
		app.NameFlag,
		app.NamespaceFlag,
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		name := app.Name(c)
		namespace := app.Namespace(c)

		if name != "" && namespace == "" {
			namespace = client.Namespace()
		}

		if name == "" {
			app := MustApplication(c.Context, client, namespace)

			name = app.Name
			namespace = app.Namespace
		}

		return applicationLogs(c.Context, client, namespace, name)
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
				go client.PodLogs(ctx, pod.Namespace, pod.Name, container.Name, os.Stdout, true)
			}
		}
	}

	<-ctx.Done()
	return nil
}
