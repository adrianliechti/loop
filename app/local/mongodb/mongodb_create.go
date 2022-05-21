package mongodb

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
			image := "mongo:5-focal"

			target := 27017
			port := app.MustPortOrRandom(c, target)

			database := "db"
			username := "root"
			password := password.MustGenerate(10, 4, 0, false, false)

			options := docker.RunOptions{
				Labels: map[string]string{
					local.KindKey: MongoDB,
				},

				Env: map[string]string{
					"MONGO_INITDB_DATABASE":      database,
					"MONGO_INITDB_ROOT_USERNAME": username,
					"MONGO_INITDB_ROOT_PASSWORD": password,
				},

				Ports: map[int]int{
					port: target,
				},

				// Volumes: map[string]string{
				// 	name: "/data/db",
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
				{"URL", fmt.Sprintf("mongodb://%s:%s@localhost:%d/%s", username, password, port, database)},
			})

			return nil
		},
	}
}
