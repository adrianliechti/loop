package dashboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
		app.KubeconfigFlag,
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

	tool, _, err := docker.Tool(ctx)

	if err != nil {
		return err
	}

	image := "kubernetesui/dashboard:v2.5.1"
	exec.CommandContext(ctx, tool, "pull", image).Run()

	args := []string{
		"run",

		"--publish",
		fmt.Sprintf("127.0.0.1:%d:%d", port, target),

		"--volume",
		client.ConfigPath() + ":/kubeconfig",

		image,

		"--metrics-provider=none",
		"--enable-skip-login",
		"--enable-insecure-login",
		"--disable-settings-authorizer",
		"--kubeconfig", "/kubeconfig",
	}

	time.AfterFunc(5*time.Second, func() {
		cli.OpenURL(fmt.Sprintf("http://localhost:%d", port))
	})

	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
