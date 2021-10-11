package tool

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var swaggerCommand = &cli.Command{
	Name:  "swagger",
	Usage: "start Swagger/OpenAPI UI",

	Flags: []cli.Flag{
		app.PortFlag,
		&cli.PathFlag{
			Name:      "path",
			Usage:     "api spec file",
			Required:  true,
			TakesFile: true,
		},
	},

	Action: func(c *cli.Context) error {
		path := c.Path("path")
		port := app.MustPortOrRandom(c, 8080)

		return runSwaggerUI(c.Context, port, path)
	},
}

func runSwaggerUI(ctx context.Context, port int, path string) error {
	dir, file := filepath.Split(path)

	target := 8080

	if port == 0 {
		port = target
	}

	image := "swaggerapi/swagger-ui"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	options := docker.RunOptions{
		Env: map[string]string{
			"SWAGGER_JSON": "/src/" + file,
		},

		Volumes: map[string]string{
			dir: "/src",
		},

		Ports: map[int]int{
			port: target,
		},
	}

	time.AfterFunc(3*time.Second, func() {
		cli.OpenURL(fmt.Sprintf("http://localhost:%d", port))
	})

	return docker.RunInteractive(ctx, image, options)
}
