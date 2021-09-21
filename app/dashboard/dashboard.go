package dashboard

import (
	"context"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
	"github.com/adrianliechti/loop/pkg/kubernetes"
)

var Command = &cli.Command{
	Name:  "dashboard",
	Usage: "run dashboard locally",

	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  app.PortFlag.Name,
			Usage: "local dashboard port",
		},
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		port := app.MustPortOrRandom(c, "9090")

		return runDashboard(c.Context, client, port)
	},
}

func runDashboard(ctx context.Context, client kubernetes.Client, port string) error {
	target := "9090"

	if port == "" {
		port = target
	}

	args := []string{
		"--metrics-provider=none",
		"--enable-skip-login",
		"--enable-insecure-login",
		"--disable-settings-authorizer",
		"--kubeconfig", "/kubeconfig",
	}

	options := docker.RunOptions{
		Ports: map[string]string{
			port: target,
		},

		Volumes: map[string]string{
			client.ConfigPath(): "/kubeconfig",
		},
	}

	return docker.RunInteractive(ctx, "kubernetesui/dashboard:v2.3.1", options, args...)
}
