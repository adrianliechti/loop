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
	"github.com/adrianliechti/loop/app/docker"
	"github.com/adrianliechti/loop/app/expose"
	"github.com/adrianliechti/loop/app/shell"
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
			config.Command,
			connect.Command,
			shell.Command,
			expose.Command,
			catapult.Command,
			dashboard.Command,
			docker.Command,

			application.Command,
		},
	}
}
