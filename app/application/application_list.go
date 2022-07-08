package application

import (
	"context"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/kubernetes/resource"
)

var listCommand = &cli.Command{
	Name:  "list",
	Usage: "list applications",

	Flags: []cli.Flag{
		app.NamespaceFlag,
		app.KubeconfigFlag,
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)
		namespace := app.Namespace(c)

		return listApplications(c.Context, client, namespace)
	},
}

func listApplications(ctx context.Context, client kubernetes.Client, namespace string) error {
	apps, err := resource.Apps(ctx, client, namespace)

	if err != nil {
		return err
	}

	rows := make([][]string, 0)
	keys := []string{"Namespace", "Name"}

	for _, a := range apps {
		rows = append(rows, []string{a.Namespace, a.Name})
	}

	cli.Table(keys, rows)
	return nil
}
