package local

import (
	"context"
	"fmt"
	"os"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var codeCommand = &cli.Command{
	Name:  "code",
	Usage: "local VS Code server",

	Flags: []cli.Flag{
		app.PortFlag,
	},

	Action: func(c *cli.Context) error {
		port := app.MustRandomPort(c, 3000)
		return startCode(c.Context, port)
	},
}

func startCode(ctx context.Context, port int) error {
	image := "adrianliechti/loop-code"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	path, err := os.Getwd()

	if err != nil {
		return err
	}

	target := 3000

	if port == 0 {
		port = target
	}

	cli.Info()

	cli.Table([]string{"Name", "Value"}, [][]string{
		{"Host", fmt.Sprintf("localhost:%d", port)},
		{"URL", fmt.Sprintf("http://localhost:%d", port)},
	})

	cli.Info()

	options := docker.RunOptions{
		Ports: map[int]int{
			port: target,
		},

		Volumes: map[string]string{
			path: "/workspace",
		},
	}

	return docker.RunInteractive(ctx, image, options)
}
