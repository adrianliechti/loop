package local

import (
	"context"
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var mailtrapCommand = &cli.Command{
	Name:  "mailtrap",
	Usage: "local MailTrap server",

	Flags: []cli.Flag{
		app.PortFlag,
	},

	Action: func(c *cli.Context) error {
		smtpPort := app.MustRandomPort(c, 2525)
		httpPort := app.MustRandomPort(c, 2580)

		return startMailTrap(c.Context, smtpPort, httpPort)
	},
}

func startMailTrap(ctx context.Context, smtpPort, httpPort int) error {
	image := "adrianliechti/loop-mailtrap"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	smtpTarget := 25
	httpTarget := 80

	if smtpPort == 0 {
		smtpPort = smtpTarget
	}

	if httpPort == 0 {
		httpPort = httpTarget
	}

	username := "admin"
	password := "notsecure"

	cli.Info()

	cli.Table([]string{"Name", "Value"}, [][]string{
		{"Host", fmt.Sprintf("localhost:%d", smtpPort)},
		{"Username", username},
		{"Password", password},
		{"URL", fmt.Sprintf("http://localhost:%d", httpPort)},
	})

	cli.Info()

	options := docker.RunOptions{
		Env: map[string]string{
			"USERNAME": username,
			"PASSWORD": password,
		},

		Ports: map[int]int{
			smtpPort: smtpTarget,
			httpPort: httpTarget,
		},
	}

	return docker.RunInteractive(ctx, image, options)
}
