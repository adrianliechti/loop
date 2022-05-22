package local

import (
	"context"
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var registryCommand = &cli.Command{
	Name:  "registry",
	Usage: "local Docker registry",

	Flags: []cli.Flag{
		app.PortFlag,
	},

	Action: func(c *cli.Context) error {
		port := app.MustPortOrRandom(c, 5000)
		return startRegistry(c.Context, port)
	},
}

func startRegistry(ctx context.Context, port int) error {
	image := "registry:2"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	target := 5000

	if port == 0 {
		port = target
	}

	cli.Info()

	cli.Table([]string{"Name", "Value"}, [][]string{
		{"Host", fmt.Sprintf("localhost:%d", port)},
		// {"Username", username},
		// {"Password", password},
		{"URL", fmt.Sprintf("http://localhost:%d", port)},
	})

	cli.Info()

	options := docker.RunOptions{
		Env: map[string]string{},

		Ports: map[int]int{
			port: target,
		},
	}

	return docker.RunInteractive(ctx, image, options)
}
