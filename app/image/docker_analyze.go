package image

import (
	"context"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var analyzeCommand = &cli.Command{
	Name:  "analyze",
	Usage: "analyze image using whaler",

	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "image",
			Required: true,
		},
		&cli.BoolFlag{
			Name:  "verbose",
			Usage: "verbose output",
		},
	},

	Action: func(c *cli.Context) error {
		image := c.String("image")

		return runWhaler(c.Context, image, c.Bool("verbose"))
	},
}

func runWhaler(ctx context.Context, image string, verbose bool) error {
	args := []string{}

	if verbose {
		args = append(args, "-v")
	}

	args = append(args, image)

	options := docker.RunOptions{
		Volumes: map[string]string{
			"/var/run/docker.sock": "/var/run/docker.sock",
		},
	}

	return docker.RunInteractive(ctx, "pegleg/whaler", options, args...)
}
