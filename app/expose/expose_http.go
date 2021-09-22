package expose

import (
	"context"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"

	corev1 "k8s.io/api/core/v1"
)

var httpCommand = &cli.Command{
	Name:  "http",
	Usage: "expose tcp server",

	Flags: []cli.Flag{
		app.NameFlag,
		app.NamespaceFlag,
		&cli.IntFlag{
			Name:     app.PortFlag.Name,
			Usage:    "local port to expose",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "host",
			Usage:    "hostname to use",
			Required: true,
		},
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		name := app.Name(c)
		namespace := app.NamespaceOrDefault(c)

		host := c.String("host")
		port := app.MustPort(c)

		return createHTTPTunnel(c.Context, client, namespace, name, host, port)
	},
}

func createHTTPTunnel(ctx context.Context, client kubernetes.Client, namespace, name, host string, port int) error {
	options := TunnelOptions{
		ServiceType: corev1.ServiceTypeClusterIP,
		ServicePorts: map[int]int{
			port: 80,
		},

		IngressHost: host,
		IngressMapping: map[string]int{
			"/": 80,
		},
	}

	return createTunnel(ctx, client, namespace, name, options)
}
