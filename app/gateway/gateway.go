package gateway

import (
	"context"
	"log/slog"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/gateway"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/system"
)

var Command = &cli.Command{
	Name:  "gateway",
	Usage: "connect ingresses/gateways",

	Flags: []cli.Flag{
		app.ScopeFlag,
		app.NamespacesFlag,
	},

	Action: func(ctx context.Context, cmd *cli.Command) error {
		client := app.MustClient(ctx, cmd)

		namespaces := app.Namespaces(ctx, cmd)

		elevated, err := system.IsElevated()

		if err != nil {
			return err
		}

		if !elevated {
			cli.Fatal("This command must be run as root!")
		}

		return StartGateway(ctx, client, namespaces)
	},
}

func StartGateway(ctx context.Context, client kubernetes.Client, namespaces []string) error {
	gateway, err := gateway.New(client, gateway.GatewayOptions{
		Namespaces: namespaces,

		Logger: slog.Default(),

		AddFunc: func(address string, hosts []string, ports []int) {
			slog.InfoContext(ctx, "adding tunnel", "address", address, "hosts", hosts, "ports", ports)
		},

		DeleteFunc: func(address string, hosts []string, ports []int) {
			slog.InfoContext(ctx, "removing tunnel", "address", address, "hosts", hosts, "ports", ports)
		},
	})

	if err != nil {
		return err
	}

	return gateway.Start(ctx)
}
