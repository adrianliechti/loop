package docker

import (
	"context"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/remote/docker"
)

var CommandConnect = &cli.Command{
	Name:  "connect",
	Usage: "connect to an existing Docker instance",

	Flags: []cli.Flag{
		app.NamespaceFlag,
	},

	Action: func(ctx context.Context, cmd *cli.Command) error {
		client := app.MustClient(ctx, cmd)

		name := cmd.Args().Get(0)
		namespace := app.Namespace(ctx, cmd)

		if name == "" {
			selected, err := selectInstance(ctx, client, namespace)

			if err != nil {
				return err
			}

			name = selected
		}

		return docker.Connect(ctx, client, name, &docker.ConnectOptions{
			Namespace: namespace,
		})
	},
}
