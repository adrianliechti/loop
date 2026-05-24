package docker

import (
	"context"
	"errors"

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

		return docker.Connect(ctx, client, name, &docker.ConnectOptions{
			Namespace: namespace,
		})
	},
}
