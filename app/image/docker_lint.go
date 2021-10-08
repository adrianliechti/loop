package image

import (
	"context"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var lintCommand = &cli.Command{
	Name:  "lint",
	Usage: "lint image using dockle",

	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "image",
			Required: true,
		},
	},

	Action: func(c *cli.Context) error {
		image := c.String("image")

		return runDockle(c.Context, image)
	},
}

func runDockle(ctx context.Context, image string) error {
	args := []string{
		// "--debug",
		image,
	}

	options := docker.RunOptions{
		Env: map[string]string{
			"DOCKER_CONTENT_TRUST": "1",
		},

		Volumes: map[string]string{
			"/var/run/docker.sock": "/var/run/docker.sock",
		},
	}

	return docker.RunInteractive(ctx, "goodwithtech/dockle", options, args...)
}
