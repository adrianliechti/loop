package local

import (
	"context"
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var mssqlCommand = &cli.Command{
	Name:  "mssql",
	Usage: "local MSSQL server",

	Flags: []cli.Flag{
		app.PortFlag,
	},

	Action: func(c *cli.Context) error {
		port := app.MustRandomPort(c, 1433)
		return startMSSQL(c.Context, port)
	},
}

func startMSSQL(ctx context.Context, port int) error {
	image := "mcr.microsoft.com/mssql/server:2019-latest"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	target := 1433

	if port == 0 {
		port = target
	}

	username := "sa"
	password := "Passw@rd"

	cli.Info()

	cli.Table([]string{"Name", "Value"}, [][]string{
		{"Host", fmt.Sprintf("localhost:%d", port)},
		{"Username", username},
		{"Password", password},
		{"URL", fmt.Sprintf("mssql://%s:%s@localhost:%d", username, password, port)},
	})

	cli.Info()

	options := docker.RunOptions{
		Env: map[string]string{
			"ACCEPT_EULA": "Y",
			"SA_PASSWORD": password,
		},

		Ports: map[int]int{
			port: target,
		},

		// Volumes: map[string]string{
		// 	name: "/var/opt/mssql",
		// },
	}

	return docker.RunInteractive(ctx, image, options)
}
