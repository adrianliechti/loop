package docker2

import (
	"context"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/remote/docker"
)

var CommandList = &cli.Command{
	Name:  "list",
	Usage: "list Docker instances",

	Action: func(ctx context.Context, cmd *cli.Command) error {
		client := app.MustClient(ctx, cmd)

		namespace := app.Namespace(ctx, cmd)

		instances, err := docker.List(ctx, client, &docker.ListOptions{
			Namespace: namespace,
		})

		if err != nil {
			return err
		}

		for _, i := range instances {
			cli.Info(i.Name)
		}

		return nil
	},
}
