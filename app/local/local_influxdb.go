package local

import (
	"context"
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var influxdbCommand = &cli.Command{
	Name:  "influxdb",
	Usage: "local InfluxDB server",

	Flags: []cli.Flag{
		app.PortFlag,
	},

	Action: func(c *cli.Context) error {
		port := app.MustRandomPort(c, 8086)
		return startInfluxDB(c.Context, port)
	},
}

func startInfluxDB(ctx context.Context, port int) error {
	image := "influxdb:2.0"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	target := 8086

	if port == 0 {
		port = target
	}

	organization := "default"
	bucket := "default"

	username := "admin"
	password := "notsecure"

	token := "00000000-0000-0000-0000-000000000000"

	cli.Info()

	cli.Table([]string{"Name", "Value"}, [][]string{
		{"Host", fmt.Sprintf("localhost:%d", port)},
		{"organization", organization},
		{"bucket", bucket},
		{"Username", username},
		{"Password", password},
		{"Token", token},
		{"URL", fmt.Sprintf("http://localhost:%d", port)},
	})

	cli.Info()

	options := docker.RunOptions{
		Env: map[string]string{
			"DOCKER_INFLUXDB_INIT_MODE": "setup",

			"DOCKER_INFLUXDB_INIT_ORG":    organization,
			"DOCKER_INFLUXDB_INIT_BUCKET": bucket,

			"DOCKER_INFLUXDB_INIT_USERNAME": username,
			"DOCKER_INFLUXDB_INIT_PASSWORD": password,

			"DOCKER_INFLUXDB_INIT_ADMIN_TOKEN": token,
		},

		Ports: map[int]int{
			port: target,
		},

		// Volumes: map[string]string{
		// 	name: "/var/lib/influxdb2",
		// },
	}

	return docker.RunInteractive(ctx, image, options)
}
