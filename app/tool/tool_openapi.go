package tool

import (
	"context"
	"path/filepath"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var openapiCommand = &cli.Command{
	Name:  "openapi",
	Usage: "generate OpenAPI client/servers",

	Flags: []cli.Flag{
		app.PortFlag,
		&cli.PathFlag{
			Name:      "path",
			Usage:     "api spec file",
			Required:  true,
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:  "generator",
			Usage: "generator to use",
		},
	},

	Action: func(c *cli.Context) error {
		path := c.Path("path")
		generator := c.String("generator")

		return runOpenAPIGenerator(c.Context, path, generator)
	},
}

func runOpenAPIGenerator(ctx context.Context, path, generator string) error {
	dir, file := filepath.Split(path)

	image := "openapitools/openapi-generator-cli"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	if generator == "" {
		return docker.RunInteractive(ctx, image, docker.RunOptions{}, "list")
	}

	options := docker.RunOptions{
		Volumes: map[string]string{
			dir: "/data",
		},
	}

	args := []string{
		"generate",
		"-g", generator,
		"-i", "/data/" + file,
		"-o", "/data/" + generator,
	}

	return docker.RunInteractive(ctx, image, options, args...)
}
