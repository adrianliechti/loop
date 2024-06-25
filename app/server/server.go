package server

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"

	"github.com/gofiber/fiber/v2"
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

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Static("/", root, fiber.Static{
		Browse: true,
		Index:  index,
	})

	if spa {
		app.Get("/*", func(ctx *fiber.Ctx) error {
			return ctx.SendFile(path.Join(root, index))
		})
	}

	go func() {
		<-ctx.Done()

		app.Shutdown()
	}()

	cli.Infof("Starting server at port %d", port)

	return app.Listen(fmt.Sprintf("127.0.0.1:%d", port))
}
