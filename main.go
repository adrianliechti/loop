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
	"github.com/adrianliechti/loop/app/proxy"
	"github.com/adrianliechti/loop/app/remote"
	"github.com/adrianliechti/loop/pkg/cli"

	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
)

var version string

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill, syscall.SIGTERM)
	defer stop()

	ctx = logr.NewContext(ctx, stdr.New(nil))

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
			proxy.Command,

			dashboard.Command,

			remote.Command,
			expose.Command,
		},
	}
}
