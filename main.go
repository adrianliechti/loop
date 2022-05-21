package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/app/application"
	"github.com/adrianliechti/loop/app/catapult"
	"github.com/adrianliechti/loop/app/config"
	"github.com/adrianliechti/loop/app/connect"
	"github.com/adrianliechti/loop/app/dashboard"
	"github.com/adrianliechti/loop/app/expose"
	"github.com/adrianliechti/loop/app/git"
	"github.com/adrianliechti/loop/app/image"
	"github.com/adrianliechti/loop/app/local/elasticsearch"
	"github.com/adrianliechti/loop/app/local/etcd"
	"github.com/adrianliechti/loop/app/local/influxdb"
	"github.com/adrianliechti/loop/app/local/kafka"
	"github.com/adrianliechti/loop/app/local/mariadb"
	"github.com/adrianliechti/loop/app/local/minio"
	"github.com/adrianliechti/loop/app/local/mongodb"
	"github.com/adrianliechti/loop/app/local/mssql"
	"github.com/adrianliechti/loop/app/local/nats"
	"github.com/adrianliechti/loop/app/local/postgres"
	"github.com/adrianliechti/loop/app/local/redis"
	"github.com/adrianliechti/loop/app/local/vault"
	"github.com/adrianliechti/loop/app/remote"
	"github.com/adrianliechti/loop/app/template"
	"github.com/adrianliechti/loop/app/tool"
	"github.com/adrianliechti/loop/pkg/cli"
)

var version string

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := initApp()

	if err := app.RunContext(ctx, os.Args); err != nil {
		cli.Fatal(err)
	}
}

func initApp() cli.App {
	return cli.App{
		Name:    "loop",
		Version: version,
		Usage:   "DevOps Loop",

		HideHelpCommand: true,

		Flags: []cli.Flag{
			app.KubeconfigFlag,
		},

		Commands: []*cli.Command{
			// Cluster
			application.Command,
			config.Command,
			connect.Command,
			catapult.Command,
			dashboard.Command,

			// Development
			{
				Name:  "local",
				Usage: "local development instances",

				Category: app.CategoryDevelopment,

				HideHelpCommand: true,

				Subcommands: []*cli.Command{
					mariadb.Command,
					postgres.Command,
					mongodb.Command,
					mssql.Command,

					etcd.Command,
					redis.Command,
					influxdb.Command,
					elasticsearch.Command,

					minio.Command,
					vault.Command,

					nats.Command,
					kafka.Command,
					// rabbitmqCommand,

					// registryCommand,
					// mailtrapCommand,

					// codeCommand,
					// grafanaCommand,
					// jupyterCommand,
				},
			},
			remote.Command,
			expose.Command,

			// Utilities
			git.Command,
			tool.Command,
			image.Command,
			template.Command,
		},
	}
}
