package local

import (
	"context"
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var mongoDBCommand = &cli.Command{
	Name:  "mongodb",
	Usage: "local MongoDB server",

	Flags: []cli.Flag{
		app.PortFlag,
	},

	Action: func(c *cli.Context) error {
		port := app.MustPortOrRandom(c, 27017)
		return startMongoDB(c.Context, port)
	},
}

func startMongoDB(ctx context.Context, port int) error {
	image := "mongo:5-focal"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	target := 27017

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
		{"URL", fmt.Sprintf("mongodb://%s:%s@localhost:%d/%s", username, password, port, database)},
	})

	cli.Info()

	options := docker.RunOptions{
		Env: map[string]string{
			"MONGO_INITDB_DATABASE":      database,
			"MONGO_INITDB_ROOT_USERNAME": username,
			"MONGO_INITDB_ROOT_PASSWORD": password,
		},

		Ports: map[int]int{
			port: target,
		},

		// Volumes: map[string]string{
		// 	name: "/data/db",
		// },
	}

	return docker.RunInteractive(ctx, image, options)
}
