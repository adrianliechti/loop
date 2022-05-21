package mssql

import (
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

func ClientCommand() *cli.Command {
	return &cli.Command{
		Name:  "cli",
		Usage: "run sqlcmd in instance",

		Action: func(c *cli.Context) error {
			ctx := c.Context
			container := local.MustContainer(ctx, MSSQL)

			options := docker.ExecOptions{
				User: "mssql",
			}

			return docker.ExecInteractive(ctx, container, options,
				"/bin/bash", "-c",
				"/opt/mssql-tools/bin/sqlcmd -S localhost -U sa -P ${SA_PASSWORD}",
			)
		},
	}
}
