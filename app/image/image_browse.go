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
		ImageFlag,
	},

	Action: func(c *cli.Context) error {
		image := MustImage(c)
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
