package image

import (
	"context"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var browseCommand = &cli.Command{
	Name:  "browse",
	Usage: "browse image using dive",

	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "image",
			Required: true,
		},
	},

	Action: func(c *cli.Context) error {
		image := c.String("image")

		return runDive(c.Context, image)
	},
}

func runDive(ctx context.Context, image string) error {
	options := docker.RunOptions{
		Volumes: map[string]string{
			"/var/run/docker.sock": "/var/run/docker.sock",
		},
	}

	return docker.RunInteractive(ctx, "wagoodman/dive", options, image)
}
