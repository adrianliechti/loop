package mssql

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
			image := "mcr.microsoft.com/mssql/server:2019-latest"
			//image := "mcr.microsoft.com/azure-sql-edge"

			target := 1433
			port := app.MustPortOrRandom(c, target)

			username := "sa"
			password := password.MustGenerate(10, 4, 0, false, false)

			options := docker.RunOptions{
				Labels: map[string]string{
					local.KindKey: MSSQL,
				},

				Env: map[string]string{
					"ACCEPT_EULA": "Y",
					"MSSQL_PID":   "Developer",
					"SA_PASSWORD": password,
				},

				// Env: map[string]string{
				// 	"ACCEPT_EULA":       "1",
				// 	"MSSQL_PID":         "Developer",
				// 	"MSSQL_SA_PASSWORD": password,
				// },

				Ports: map[int]int{
					port: target,
				},

				// Volumes: map[string]string{
				// 	name: "/var/opt/mssql",
				// },
			}

			if err := docker.Run(ctx, image, options); err != nil {
				return err
			}

			cli.Table([]string{"Name", "Value"}, [][]string{
				{"Host", fmt.Sprintf("localhost:%d", port)},
				{"Username", username},
				{"Password", password},
				{"URL", fmt.Sprintf("mssql://%s:%s@localhost:%d", username, password, port)},
			})

			return nil
		},
	}
}
