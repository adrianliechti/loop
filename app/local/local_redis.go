package local

import (
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
	"github.com/sethvargo/go-password/password"
)

const (
	Redis = "redis"
)

var redisCommand = &cli.Command{
	Name:  Redis,
	Usage: "local Redis server",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		listCommand(Redis),

		createRedis(),
		deleteCommand(Redis),

		logsCommand(Redis),
		shellCommand(Redis, "/bin/bash"),
		clientRedis(),
	},
}

func createRedis() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "create instance",

		Flags: []cli.Flag{
			app.PortFlag,
		},

		Action: func(c *cli.Context) error {
			ctx := c.Context
			image := "redis:6-bullseye"

			target := 6379
			port := app.MustPortOrRandom(c, target)

			password, err := password.Generate(10, 4, 0, false, false)

			if err != nil {
				return err
			}

			options := docker.RunOptions{
				Labels: map[string]string{
					KindKey: Redis,
				},

				Env: map[string]string{
					"REDIS_PASSWORD": password,
				},

				Ports: map[int]int{
					port: target,
				},

				// Volumes: map[string]string{
				// 	name: "/data",
				// },
			}

			if err := docker.Run(ctx, image, options); err != nil {
				return err
			}

			cli.Table([]string{"Name", "Value"}, [][]string{
				{"Host", fmt.Sprintf("localhost:%d", port)},
				{"Password", password},
				{"URL", fmt.Sprintf("redis://:%s@localhost:%d", password, port)},
			})

			return nil
		},
	}
}

func clientRedis() *cli.Command {
	return &cli.Command{
		Name:  "cli",
		Usage: "run redis-cli in instance",

		Action: func(c *cli.Context) error {
			ctx := c.Context
			container := mustContainer(ctx, Redis)

			options := docker.ExecOptions{
				User: "redis",
			}

			return docker.ExecInteractive(ctx, container, options,
				"/bin/bash", "-c",
				"redis-cli",
			)
		},
	}
}
