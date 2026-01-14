package docker

import (
	"context"
	"errors"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/remote/docker"
)

var CommandDelete = &cli.Command{
	Name:  "delete",
	Usage: "delete a Docker instance",

	Action: func(ctx context.Context, cmd *cli.Command) error {
		client := app.MustClient(ctx, cmd)

		name := cmd.Args().Get(0)
		namespace := app.Namespace(ctx, cmd)

		if namespace == "" {
			namespace = client.Namespace()
		}

		if name == "" {
			candidates, err := docker.List(ctx, client, &docker.ListOptions{
				Namespace: namespace,
			})

			if err != nil {
				return err
			}

			if len(candidates) == 0 {
				return errors.New("no Docker instances found")
			}

			var items []string

			for _, c := range candidates {
				items = append(items, c.Name)
			}

			_, name = cli.MustSelect("Daemon", items)
		}

		if err := docker.Delete(ctx, client, namespace, name); err != nil {
			return err
		}

		return nil
	},
}
