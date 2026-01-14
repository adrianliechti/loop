package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/app/bridge"
	"github.com/adrianliechti/loop/app/build"
	"github.com/adrianliechti/loop/app/code"
	"github.com/adrianliechti/loop/app/connect"
	"github.com/adrianliechti/loop/app/docker"
	"github.com/adrianliechti/loop/app/docker2"
	"github.com/adrianliechti/loop/app/expose"
	"github.com/adrianliechti/loop/app/prism"
	"github.com/adrianliechti/loop/app/run"
	"github.com/adrianliechti/loop/app/toolkit"

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

	if err := app.Run(ctx, os.Args); err != nil {
		cli.Fatal(err)
	}
}

func initApp() cli.Command {
	return cli.Command{
		Usage: "Loop",

		Suggest: true,
		Version: version,

		HideHelpCommand: true,

		Flags: []cli.Flag{
			app.KubeconfigFlag,
		},

		Commands: []*cli.Command{
			bridge.Command,

			connect.Command,
			expose.Command,

			run.Command,

			code.Command,
			prism.Command,
			toolkit.Command,

			build.Command,
			docker.Command,
			docker2.Command,
		},
	}
}
