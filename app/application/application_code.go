package application

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
	"github.com/adrianliechti/loop/pkg/kubernetes"
)

var codeCommand = &cli.Command{
	Name:  "code",
	Usage: "run VSCode Server locally",

	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  app.PortFlag.Name,
			Usage: "local server port",
		},
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		port := app.MustPortOrRandom(c, 3000)

		return runCode(c.Context, client, port)
	},
}

func runCode(ctx context.Context, client kubernetes.Client, port int) error {
	wd, err := os.Getwd()

	if err != nil {
		return err
	}

	target := 3000

	if port == 0 {
		port = target
	}

	args := []string{}

	options := docker.RunOptions{
		Ports: map[int]int{
			port: target,
		},

		Volumes: map[string]string{
			wd: "/workspace",
		},
	}

	docker.Pull(ctx, "adrianliechti/loop-code")

	time.AfterFunc(5*time.Second, func() {
		cli.OpenURL(fmt.Sprintf("http://localhost:%d", port))
	})

	return docker.RunInteractive(ctx, "adrianliechti/loop-code", options, args...)
}
