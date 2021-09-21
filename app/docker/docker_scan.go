package docker

import (
	"context"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var scanCommand = &cli.Command{
	Name:  "scan",
	Usage: "scan image using trivy",

	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "image",
			Required: true,
		},
	},

	Action: func(c *cli.Context) error {
		image := c.String("image")

		return runTrivy(c.Context, image)
	},
}

func runTrivy(ctx context.Context, image string) error {
	options := docker.RunOptions{
		Env: map[string]string{},

		Volumes: map[string]string{
			"trivy-cache": "/root/.cache/",
		},
	}

	return docker.RunInteractive(ctx, "aquasec/trivy", options, image)
}
