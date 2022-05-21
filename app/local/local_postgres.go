package local

import (
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
	"github.com/sethvargo/go-password/password"
)

const (
	PostgreSQL = "postgres"
)

var postgresCommand = &cli.Command{
	Name:  PostgreSQL,
	Usage: "local PostgreSQL server",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		listCommand(PostgreSQL),

		createPostgreSQL(),
		deleteCommand(PostgreSQL),

		logsCommand(PostgreSQL),
		shellCommand(PostgreSQL, "/bin/bash"),
		clientPostgreSQL(),
	},
}

func createPostgreSQL() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "create instance",

		Flags: []cli.Flag{
			app.PortFlag,
		},

		Action: func(c *cli.Context) error {
			ctx := c.Context
			image := "postgres:14-bullseye"

			target := 5432
			port := app.MustPortOrRandom(c, target)

			database := "postgres"
			username := "postgres"
			password, err := password.Generate(10, 4, 0, false, false)

			if err != nil {
				return err
			}

			options := docker.RunOptions{
				Labels: map[string]string{
					KindKey: PostgreSQL,
				},

				Env: map[string]string{
					"POSTGRES_DB":       database,
					"POSTGRES_USER":     username,
					"POSTGRES_PASSWORD": password,
				},

				Ports: map[int]int{
					port: target,
				},

				// Volumes: map[string]string{
				// 	name: "/var/lib/postgresql/data",
				// },
			}

			if err := docker.Run(ctx, image, options); err != nil {
				return err
			}

			cli.Table([]string{"Name", "Value"}, [][]string{
				{"Host", fmt.Sprintf("localhost:%d", port)},
				{"database", database},
				{"Username", username},
				{"Password", password},
				{"URL", fmt.Sprintf("postgresql://%s:%s@localhost:%d/%s", username, password, port, database)},
			})

			return nil
		},
	}
}

func clientPostgreSQL() *cli.Command {
	return &cli.Command{
		Name:  "cli",
		Usage: "run psql in instance",

		Action: func(c *cli.Context) error {
			ctx := c.Context
			container := mustContainer(ctx, PostgreSQL)

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
