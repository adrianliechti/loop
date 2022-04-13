package tool

import (
	"context"
	"os"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var clocCommand = &cli.Command{
	Name:  "cloc",
	Usage: "count lines of code",

	Action: func(c *cli.Context) error {
		return runCloc(c.Context)
	},
}

func runCloc(ctx context.Context) error {
	wd, err := os.Getwd()

	if err != nil {
		return err
	}

	image := "aldanial/cloc"

	// if err := docker.Pull(ctx, image); err != nil {
	// 	return err
	// }

	args := []string{
		"--quiet",
		"--hide-rate",
		"/src",
	}

	options := docker.RunOptions{
		Volumes: map[string]string{
			wd: "/src",
		},
	}

	return docker.RunInteractive(ctx, image, options, args...)
}
