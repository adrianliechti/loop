package local

import (
	"context"
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var grafanaCommand = &cli.Command{
	Name:  "grafana",
	Usage: "local Grafana server",

	Flags: []cli.Flag{
		app.PortFlag,
	},

	Action: func(c *cli.Context) error {
		port := app.MustRandomPort(c, 3000)
		return startGrafana(c.Context, port)
	},
}

func startGrafana(ctx context.Context, port int) error {
	image := "grafana/grafana"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	target := 3000

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
		{"URL", fmt.Sprintf("http://localhost:%d", port)},
	})

	cli.Info()

	options := docker.RunOptions{
		Env: map[string]string{

			"GF_SECURITY_ADMIN_USER":     username,
			"GF_SECURITY_ADMIN_PASSWORD": password,
		},

		Ports: map[int]int{
			port: target,
		},

		// Volumes: map[string]string{
		// 	name: "/var/lib/grafana",
		// },
	}

	return docker.RunInteractive(ctx, image, options)
}
