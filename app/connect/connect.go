package connect

import (
	"context"
	"errors"
	"log/slog"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/catapult"
	"github.com/adrianliechti/loop/pkg/gateway"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/system"
)

var Command = &cli.Command{
	Name:  "connect",
	Usage: "connect Kubernetes network",

	Flags: []cli.Flag{
		app.ScopeFlag,
		app.NamespacesFlag,
	},

	Action: func(ctx context.Context, cmd *cli.Command) error {
		client := app.MustClient(ctx, cmd)

		scope := app.Scope(ctx, cmd)
		namespaces := app.Namespaces(ctx, cmd)

		elevated, err := system.IsElevated()

		if err != nil {
			return err
		}

		if !elevated {
			cli.Fatal("This command must be run as root!")
		}

		return Connect(ctx, client, namespaces, scope)
	},
}

func Connect(ctx context.Context, client kubernetes.Client, namespaces []string, scope string) error {
	if scope == "" && len(namespaces) > 0 {
		scope = namespaces[0]
	}

	if scope == "" {
		scope = client.Namespace()
	}

	catapult, err := catapult.New(client, catapult.CatapultOptions{
		Scope:      scope,
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

	// Share a cancellable context so the first failure tears down the other
	// side too; otherwise an early gateway error would block on the catapult
	// goroutine until the user interrupts the whole command.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errs := make(chan error, 2)

	go func() {
		errs <- catapult.Start(ctx)
	}()

	go func() {
		errs <- gateway.Start(ctx)
	}()

	first := <-errs
	cancel()
	second := <-errs

	return errors.Join(first, second)
}
