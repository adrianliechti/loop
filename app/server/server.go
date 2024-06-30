package server

import (
	"context"
	"fmt"
	"os"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

var Command = &cli.Command{
	Name:  "server",
	Usage: "start local web server",

	Flags: []cli.Flag{
		app.PortFlag,

		&cli.BoolFlag{
			Name:  "spa",
			Usage: "enable SPA redirect",
		},

		&cli.StringFlag{
			Name:  "index",
			Usage: "index file name",
			Value: "index.html",
		},
	},

	Action: func(c *cli.Context) error {
		port := app.MustPortOrRandom(c, 3000)

		spa := c.Bool("spa")
		index := c.String("index")

		if index == "" {
			index = "index.html"
		}

		return startWebServer(c.Context, port, index, spa)
	},
}

func startWebServer(ctx context.Context, port int, index string, spa bool) error {
	root, err := os.Getwd()

	if err != nil {
		return err
	}

	if port == 0 {
		port = 3000
	}

	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.Use(middleware.StaticWithConfig(middleware.StaticConfig{
		Root:  root,
		HTML5: spa,
	}))

	go func() {
		<-ctx.Done()

		e.Shutdown(context.Background())
	}()

	return e.Start(fmt.Sprintf("127.0.0.1:%d", port))
}
