package minio

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
			image := "minio/minio"

			apiPort := app.MustPortOrRandom(c, 9000)
			consolePort := app.MustRandomPort(c, apiPort+1)

			username := "root"
			password, err := password.Generate(10, 4, 0, false, false)

			if err != nil {
				return err
			}

			options := docker.RunOptions{
				Labels: map[string]string{
					local.KindKey: MinIO,
				},

				Env: map[string]string{
					"MINIO_ROOT_USER":     username,
					"MINIO_ROOT_PASSWORD": password,
				},

				Ports: map[int]int{
					apiPort:     9000,
					consolePort: 9001,
				},

				// Volumes: map[string]string{
				// 	path: "/data",
				// },
			}

			if err := docker.Run(ctx, image, options, "server", "/data", "--console-address", ":9001"); err != nil {
				return err
			}

			cli.Table([]string{"Name", "Value"}, [][]string{
				{"Host", fmt.Sprintf("localhost:%d", apiPort)},
				{"Username", username},
				{"Password", password},
				{"API", fmt.Sprintf("http://localhost:%d", apiPort)},
				{"Console", fmt.Sprintf("http://localhost:%d", consolePort)},
			})

			return nil
		},
	}
}
