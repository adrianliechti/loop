package expose

import (
	"context"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/to"

	corev1 "k8s.io/api/core/v1"
)

var tcpCommand = &cli.Command{
	Name:  "tcp",
	Usage: "expose tcp server",

	Flags: []cli.Flag{
		app.NameFlag,
		app.NamespaceFlag,
		&cli.IntSliceFlag{
			Name:     app.PortsFlag.Name,
			Usage:    "local port(s) to expose",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "host",
			Usage: "hostname to use (needs External-DNS)",
		},
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		name := to.String(app.Name(c))
		namespace := app.Namespace(c)

		if namespace == nil {
			namespace = to.StringPtr(client.Namespace())
		}

		host := c.String("host")
		ports := app.MustPorts(c)

		return createTCPTunnel(c.Context, client, *namespace, name, host, ports)
	},
}

func createTCPTunnel(ctx context.Context, client kubernetes.Client, namespace, name, host string, ports []int) error {
	mapping := map[int]int{}

	for _, p := range ports {
		mapping[p] = p
	}

	options := TunnelOptions{

		ServiceType:  corev1.ServiceTypeLoadBalancer,
		ServiceHost:  host,
		ServicePorts: mapping,
	}

	return createTunnel(ctx, client, namespace, name, options)
}
