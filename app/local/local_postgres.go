package local

import (
	"context"
	"fmt"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var postgresCommand = &cli.Command{
	Name:  "postgres",
	Usage: "local PostgreSQL server",

	Flags: []cli.Flag{},

	Action: func(c *cli.Context) error {
		return startPostgreSQL(c.Context, 0)
	},
}

func startPostgreSQL(ctx context.Context, port int) error {
	image := "postgres:14-bullseye"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	target := 5432

	if port == 0 {
		port = target
	}

	database := "postgres"
	username := "postgres"
	password := "notsecure"

	cli.Info()

	cli.Table([]string{"Name", "Value"}, [][]string{
		{"Host", fmt.Sprintf("localhost:%d", port)},
		{"database", database},
		{"Username", username},
		{"Password", password},
		{"URL", fmt.Sprintf("postgresql://%s:%s@localhost:%d/%s", username, password, port, database)},
	})

	cli.Info()

	options := docker.RunOptions{
		Env: map[string]string{
			"POSTGRES_DB":       database,
			"POSTGRES_USER":     username,
			"POSTGRES_PASSWORD": password,
		},

		Ports: map[int]int{
			port: target,
		},

		// Volumes: map[string]string{
		// 	name: "/var/lib/postgresql/data",
		// },
	}

	return docker.RunInteractive(ctx, image, options)
}
