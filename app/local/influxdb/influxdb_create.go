package influxdb

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
			image := "influxdb:2.2"

			target := 8086
			port := app.MustPortOrRandom(c, target)

			organization := "default"
			bucket := "default"

			username := "admin"

			token, err := password.Generate(10, 4, 0, false, false)

			if err != nil {
				return err
			}

			password, err := password.Generate(10, 4, 0, false, false)

			if err != nil {
				return err
			}

			options := docker.RunOptions{
				Labels: map[string]string{
					local.KindKey: InfluxDB,
				},

				Env: map[string]string{
					"DOCKER_INFLUXDB_INIT_MODE": "setup",

					"DOCKER_INFLUXDB_INIT_ORG":    organization,
					"DOCKER_INFLUXDB_INIT_BUCKET": bucket,

					"DOCKER_INFLUXDB_INIT_USERNAME": username,
					"DOCKER_INFLUXDB_INIT_PASSWORD": password,

					"DOCKER_INFLUXDB_INIT_ADMIN_TOKEN": token,
				},

				Ports: map[int]int{
					port: target,
				},

				// Volumes: map[string]string{
				// 	name: "/var/lib/influxdb2",
				// },
			}

			if err := docker.Run(ctx, image, options); err != nil {
				return err
			}

			cli.Table([]string{"Name", "Value"}, [][]string{
				{"Host", fmt.Sprintf("localhost:%d", port)},
				{"organization", organization},
				{"bucket", bucket},
				{"Username", username},
				{"Password", password},
				{"Token", token},
				{"URL", fmt.Sprintf("http://localhost:%d", port)},
			})

			return nil
		},
	}
}
