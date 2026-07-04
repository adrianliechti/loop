package docker

import (
	"context"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/remote/docker"
)

var CommandDelete = &cli.Command{
	Name:  "delete",
	Usage: "delete a Docker instance",

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

		return docker.Delete(ctx, client, namespace, name)
	},
}
