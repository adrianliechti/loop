package postgres

import (
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

func ClientCommand() *cli.Command {
	return &cli.Command{
		Name:  "cli",
		Usage: "run psql in instance",

		Action: func(c *cli.Context) error {
			ctx := c.Context
			container := local.MustContainer(ctx, PostgreSQL)

			options := docker.ExecOptions{
				User: "postgres",
			}

			return docker.ExecInteractive(ctx, container, options,
				"/bin/bash", "-c",
				"psql --username ${POSTGRES_USER} --dbname ${POSTGRES_DB}",
			)
		},
	}
}
