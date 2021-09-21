package application

import (
	"context"
	"os"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var packCommand = &cli.Command{
	Name:  "pack",
	Usage: "create Docker image",

	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "image",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "builder",
			Usage: "builder image",

			Value: "gcr.io/buildpacks/builder",
		},
	},

	Action: func(c *cli.Context) error {
		image := c.String("image")
		builder := c.String("builder")

		return runPack(c.Context, image, builder)
	},
}

func runPack(ctx context.Context, image, builder string) error {
	wd, err := os.Getwd()

	if err != nil {
		return err
	}

	args := []string{
		"build",
		image,
		"--path", "/src",
		"--builder", builder,
	}

	options := docker.RunOptions{
		User: "0:0",
		Volumes: map[string]string{
			wd: "/src",

			"/var/run/docker.sock": "/var/run/docker.sock",
		},
	}

	return docker.RunInteractive(ctx, "buildpacksio/pack", options, args...)
}
