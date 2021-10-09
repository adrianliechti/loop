package local

import (
	"context"
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var jupyterCommand = &cli.Command{
	Name:  "jupyter",
	Usage: "local Jupyter server",

	Flags: []cli.Flag{
		app.PortFlag,
	},

	Action: func(c *cli.Context) error {
		port := app.MustRandomPort(c, 8888)
		return startJupyter(c.Context, port)
	},
}

func startJupyter(ctx context.Context, port int) error {
	image := "jupyter/datascience-notebook"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	target := 8888

	if port == 0 {
		port = target
	}

	token := "notsecure"

	cli.Info()

	cli.Table([]string{"Name", "Value"}, [][]string{
		{"Host", fmt.Sprintf("localhost:%d", port)},
		{"Token", token},
		{"URL", fmt.Sprintf("http://localhost:%d?token=%s", port, token)},
	})

	cli.Info()

	options := docker.RunOptions{
		Env: map[string]string{
			"JUPYTER_TOKEN":      token,
			"JUPYTER_ENABLE_LAB": "yes",
			"RESTARTABLE":        "yes",
		},

		Ports: map[int]int{
			port: target,
		},

		// Volumes: map[string]string{
		// 	name: "/home/jovyan/work",
		// },
	}

	return docker.RunInteractive(ctx, image, options)
}
