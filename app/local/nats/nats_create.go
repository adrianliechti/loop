package nats

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
			image := "nats:2-linux"

			target := 4222
			port := app.MustPortOrRandom(c, target)

			username := "admin"
			password := password.MustGenerate(10, 4, 0, false, false)

			options := docker.RunOptions{
				Labels: map[string]string{
					local.KindKey: NATS,
				},

				Env: map[string]string{
					"USERNAME": username,
					"PASSWORD": password,
				},

				Ports: map[int]int{
					port: target,
				},
			}

			args := []string{
				"-js",
				"--name", "default",
				"--cluster_name", "default",
				"--user", username,
				"--pass", password,
			}

			if err := docker.Run(ctx, image, options, args...); err != nil {
				return err
			}

			cli.Table([]string{"Name", "Value"}, [][]string{
				{"Host", fmt.Sprintf("localhost:%d", port)},
				{"Username", username},
				{"Password", password},
				{"URL", fmt.Sprintf("nats://%s:%s@localhost:%d", username, password, port)},
			})

			return nil
		},
	}
}
