package redis

import (
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

func ClientCommand() *cli.Command {
	return &cli.Command{
		Name:  "cli",
		Usage: "run redis-cli in instance",

		Action: func(c *cli.Context) error {
			ctx := c.Context
			container := local.MustContainer(ctx, Redis)

			options := docker.ExecOptions{
				User: "redis",
			}

			return docker.ExecInteractive(ctx, container, options,
				"/bin/bash", "-c",
				"redis-cli",
			)
		},
	}
}
