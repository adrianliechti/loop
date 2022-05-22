package dashboard

import (
	"context"
	"fmt"
	"time"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
	"github.com/adrianliechti/loop/pkg/kubernetes"
)

var Command = &cli.Command{
	Name:  "dashboard",
	Usage: "start Kubernetes Dashboard",

	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  app.PortFlag.Name,
			Usage: "local dashboard port",
		},
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		port := app.MustPortOrRandom(c, 9090)

		return runDashboard(c.Context, client, port)
	},
}

func runDashboard(ctx context.Context, client kubernetes.Client, port int) error {
	target := 9090

	if port == 0 {
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
		Ports: map[int]int{
			port: target,
		},

		Volumes: map[string]string{
			client.ConfigPath(): "/kubeconfig",
		},
	}

	image := "kubernetesui/dashboard:v2.5.1"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	time.AfterFunc(5*time.Second, func() {
		cli.OpenURL(fmt.Sprintf("http://localhost:%d", port))
	})

	return docker.RunInteractive(ctx, image, options, args...)
}
