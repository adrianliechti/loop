package image

import (
	"context"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var bomCommand = &cli.Command{
	Name:  "bom",
	Usage: "bill of material using syft",

	Flags: []cli.Flag{
		ImageFlag,
	},

	Action: func(c *cli.Context) error {
		image := MustImage(c)
		return runSyft(c.Context, image)
	},
}

func runSyft(ctx context.Context, image string) error {
	args := []string{
		"-o", "table",
	}

	args = append(args, image)

	options := docker.RunOptions{
		Volumes: map[string]string{
			"/var/run/docker.sock": "/var/run/docker.sock",
		},
	}

	return docker.RunInteractive(ctx, "anchore/syft", options, args...)
}
