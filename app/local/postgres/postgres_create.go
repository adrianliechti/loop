package postgres

import (
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"

	"github.com/sethvargo/go-password/password"
)

func CreateCommand() *cli.Command {
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
			password := password.MustGenerate(10, 4, 0, false, false)

			options := docker.RunOptions{
				Labels: map[string]string{
					local.KindKey: PostgreSQL,
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
