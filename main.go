package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/app/application"
	"github.com/adrianliechti/loop/app/code"
	"github.com/adrianliechti/loop/app/connect"
	"github.com/adrianliechti/loop/app/container"
	"github.com/adrianliechti/loop/app/docker"
	"github.com/adrianliechti/loop/app/expose"
	"github.com/adrianliechti/loop/app/jupyter"
	"github.com/adrianliechti/loop/app/toolkit"
	"github.com/adrianliechti/loop/pkg/cli"

	"github.com/lmittmann/tint"
)

var version string

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill, syscall.SIGTERM)
	defer stop()

	slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, &tint.Options{
		Level:      slog.LevelInfo,
		TimeFormat: time.Kitchen,
	})))

	app := initApp()

	if err := app.RunContext(ctx, os.Args); err != nil {
		cli.Fatal(err)
	}
}

func initApp() cli.App {
	return cli.App{
		Usage: "Loop",

		Suggest: true,
		Version: version,

		HideHelpCommand: true,

		Flags: []cli.Flag{
			app.KubeconfigFlag,
		},

		Commands: []*cli.Command{
			application.Command,

			container.Command,
			toolkit.Command,

			code.Command,
			jupyter.Command,

			connect.Command,
			docker.Command,

			expose.Command,
		},
	}
}
