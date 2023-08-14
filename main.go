package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adrianliechti/loop/app/application"
	"github.com/adrianliechti/loop/app/connect"
	"github.com/adrianliechti/loop/app/expose"
	"github.com/adrianliechti/loop/app/remote"
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

		Commands: []*cli.Command{
			application.Command,

			connect.Command,

			remote.Command,
			expose.Command,
		},
	}
}
