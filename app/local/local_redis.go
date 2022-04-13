package local

import (
	"context"
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var redisCommand = &cli.Command{
	Name:  "redis",
	Usage: "local Redis server",

	Flags: []cli.Flag{
		app.PortFlag,
	},

	Action: func(c *cli.Context) error {
		port := app.MustPortOrRandom(c, 6379)
		return startRedis(c.Context, port)
	},
}

func startRedis(ctx context.Context, port int) error {
	image := "redis:6-bullseye"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	target := 6379

	if port == 0 {
		port = target
	}

	password := "notsecure"

	cli.Info()

	cli.Table([]string{"Name", "Value"}, [][]string{
		{"Host", fmt.Sprintf("localhost:%d", port)},
		{"Password", password},
		{"URL", fmt.Sprintf("redis://:%s@localhost:%d", password, port)},
	})

	cli.Info()

	options := docker.RunOptions{
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

	return docker.RunInteractive(ctx, image, options)
}
