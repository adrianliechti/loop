package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/adrianliechti/loop/app/application"
	"github.com/adrianliechti/loop/app/catapult"
	"github.com/adrianliechti/loop/app/connect"
	"github.com/adrianliechti/loop/app/dashboard"
	"github.com/adrianliechti/loop/app/expose"
	"github.com/adrianliechti/loop/app/remote"
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
		Version: version,
		Usage:   "DevOps Loop",

		HideHelpCommand: true,

		Commands: []*cli.Command{
			application.Command,
			connect.Command,
			catapult.Command,
			dashboard.Command,

			remote.Command,
			expose.Command,
		},
	}
}
