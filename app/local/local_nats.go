package local

import (
	"context"
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var natsCommand = &cli.Command{
	Name:  "nats",
	Usage: "local NATS server",

	Flags: []cli.Flag{
		app.PortFlag,
	},

	Action: func(c *cli.Context) error {
		port := app.MustPortOrRandom(c, 4222)
		return startNATS(c.Context, port)
	},
}

func startNATS(ctx context.Context, port int) error {
	image := "nats:2-linux"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	target := 4222

	if port == 0 {
		port = target
	}

	username := "admin"
	password := "notsecure"

	cli.Info()

	cli.Table([]string{"Name", "Value"}, [][]string{
		{"Host", fmt.Sprintf("localhost:%d", port)},
		{"Username", username},
		{"Password", password},
		{"URL", fmt.Sprintf("nats://%s:%s@localhost:%d", username, password, port)},
	})

	cli.Info()

	options := docker.RunOptions{
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

	return docker.RunInteractive(ctx, image, options, args...)
}
