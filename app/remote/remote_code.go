package remote

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/system"
)

var codeCommand = &cli.Command{
	Name:  "code",
	Usage: "run cluster VSCode Server",

	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  app.PortFlag.Name,
			Usage: "local server port",
		},

		// &cli.StringSliceFlag{
		// 	Name:  app.PortsFlag.Name,
		// 	Usage: "forwarded ports",
		// },
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		path, err := os.Getwd()

		if err != nil {
			return err
		}

		port := app.MustPortOrRandom(c, 3000)

		return runCode(c.Context, client, path, port, nil)
	},
}

func runCode(ctx context.Context, client kubernetes.Client, path string, port int, ports map[int]int) error {
	containerPort, err := system.FreePort(0)

	if err != nil {
		return err
	}

	cli.Infof("Starting helper container...")
	container, err := startServer(ctx, path, containerPort)

	if err != nil {
		return err
	}

	defer func() {
		cli.Infof("Stopping helper container (%s)...", container)
		stopServer(context.Background(), container)
	}()

	namespace := "default"

	cli.Infof("Starting remote VSCode...")
	pod, err := startPod(ctx, client, namespace, "adrianliechti/loop-code", false, false)

	if err != nil {
		return err
	}

	defer func() {
		cli.Infof("Stopping remote VSCode (%s/%s)...", namespace, pod)
		stopPod(context.Background(), client, namespace, pod)
	}()

	time.AfterFunc(5*time.Second, func() {
		cli.OpenURL(fmt.Sprintf("http://localhost:%d", port))
	})

	tunnelPorts := map[int]int{
		port: 3000,
	}

	cli.Info("Press ctrl-c to stop remote VSCode server")

	return runTunnel(ctx, client, namespace, pod, containerPort, tunnelPorts)
}
