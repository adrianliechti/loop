package mariadb

import (
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

func ClientCommand() *cli.Command {
	return &cli.Command{
		Name:  "cli",
		Usage: "run mysql in instance",

		Action: func(c *cli.Context) error {
			ctx := c.Context
			container := local.MustContainer(ctx, MariaDB)

			options := docker.ExecOptions{}

			return docker.ExecInteractive(ctx, container, options,
				"/bin/bash", "-c",
				"mysql --user=root --password=${MARIADB_ROOT_PASSWORD} ${MARIADB_DATABASE}",
			)
		},
	}
}
