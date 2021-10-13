package tool

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"

	"github.com/gofiber/fiber/v2"
)

var webCommand = &cli.Command{
	Name:  "web",
	Usage: "local Web server",

	Flags: []cli.Flag{
		app.PortFlag,
		&cli.BoolFlag{
			Name:  "spa",
			Usage: "enable SPA redirect",
		},
	},

	Action: func(c *cli.Context) error {
		port := app.MustPortOrRandom(c, 3000)
		spa := c.Bool("spa")

		return startWebServer(c.Context, port, spa)
	},
}

func startWebServer(ctx context.Context, port int, spa bool) error {
	root, err := os.Getwd()

	if err != nil {
		return err
	}

	index := "index.html"

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
