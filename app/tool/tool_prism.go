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

var prismCommand = &cli.Command{
	Name:  "prism",
	Usage: "start Prism API mock server",

	Flags: []cli.Flag{
		app.PortFlag,
		&cli.PathFlag{
			Name:      "path",
			Usage:     "api spec file",
			Required:  true,
			TakesFile: true,
		},
		&cli.BoolFlag{
			Name:  "dynamic",
			Usage: "dynamic data generation",
		},
	},

	Action: func(c *cli.Context) error {
		path := c.Path("path")
		port := app.MustPortOrRandom(c, 4010)

		dynamic := c.Bool("dynamic")

		return runPrism(c.Context, port, path, dynamic)
	},
}

func runPrism(ctx context.Context, port int, path string, dynamic bool) error {
	dir, file := filepath.Split(path)

	target := 4010

	if port == 0 {
		port = target
	}

	image := "stoplight/prism:4"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	options := docker.RunOptions{
		Volumes: map[string]string{
			dir: "/data",
		},

		Ports: map[int]int{
			port: target,
		},
	}

	args := []string{
		"mock",
		"-h", "0.0.0.0",
	}

	if dynamic {
		args = append(args, "-d")
	}

	args = append(args, "/data/"+file)

	time.AfterFunc(3*time.Second, func() {
		cli.OpenURL(fmt.Sprintf("http://localhost:%d", port))
	})

	return docker.RunInteractive(ctx, image, options, args...)
}
