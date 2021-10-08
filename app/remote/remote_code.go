package remote

import (
	"context"
	"os"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
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

		port := app.MustPortOrRandom(c, 3000)

		serverPort := app.MustRandomPort(c, 0)
		serverPath, err := os.Getwd()

		if err != nil {
			return err
		}

		ports := map[int]int{
			port: 3000,
		}

		return runCode(c.Context, client, serverPath, serverPort, ports)
	},
}

func runCode(ctx context.Context, client kubernetes.Client, path string, port int, ports map[int]int) error {
	container, err := startServer(ctx, path, port)

	if err != nil {
		return err
	}

	defer stopServer(context.Background(), container)

	namespace := "default"

	pod, err := startPod(ctx, client, namespace, "adrianliechti/loop-code", false, false)

	if err != nil {
		return err
	}

	defer stopPod(context.Background(), client, namespace, pod)

	return runTunnel(ctx, client, namespace, pod, port, ports)
}
