package local

import (
	"context"
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var mariadbCommand = &cli.Command{
	Name:  "mariadb",
	Usage: "local MariaDB server",

	Flags: []cli.Flag{
		app.PortFlag,
	},

	Action: func(c *cli.Context) error {
		port := app.MustRandomPort(c, 3306)
		return startMariaDB(c.Context, port)
	},
}

func startMariaDB(ctx context.Context, port int) error {
	image := "mariadb:10.6-focal"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	target := 3306

	if port == 0 {
		port = target
	}

	database := "db"
	username := "root"
	password := "notsecure"

	cli.Info()

	cli.Table([]string{"Name", "Value"}, [][]string{
		{"Host", fmt.Sprintf("localhost:%d", port)},
		{"database", database},
		{"Username", username},
		{"Password", password},
		{"URL", fmt.Sprintf("mariadb://%s:%s@localhost:%d/%s", username, password, port, database)},
	})

	cli.Info()

	options := docker.RunOptions{
		Env: map[string]string{
			"MARIADB_DATABASE":      database,
			"MARIADB_ROOT_PASSWORD": password,
		},

		Ports: map[int]int{
			port: target,
		},

		// Volumes: map[string]string{
		// 	name: "/var/lib/mysql",
		// },
	}

	return docker.RunInteractive(ctx, image, options)
}
