package tool

import (
	"context"
	"fmt"
	"time"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var hoppscotchCommand = &cli.Command{
	Name:  "hoppscotch",
	Usage: "start Hoppscotch UI",

	Flags: []cli.Flag{
		app.PortFlag,
	},

	Action: func(c *cli.Context) error {
		port := app.MustPortOrRandom(c, 3000)

		return runHoppscotch(c.Context, port)
	},
}

func runHoppscotch(ctx context.Context, port int) error {
	target := 3000

	if port == 0 {
		port = target
	}

	image := "hoppscotch/hoppscotch"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	options := docker.RunOptions{
		Ports: map[int]int{
			port: target,
		},
	}

	time.AfterFunc(3*time.Second, func() {
		cli.OpenURL(fmt.Sprintf("http://localhost:%d", port))
	})

	return docker.RunInteractive(ctx, image, options)
}
