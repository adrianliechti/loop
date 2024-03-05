package connect

import (
	"context"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/kubernetes/catapult"
	"github.com/adrianliechti/loop/pkg/system"
)

var catapultCommand = &cli.Command{
	Name:  "catapult",
	Usage: "connect services using port bulk-forwarding",

	Flags: []cli.Flag{
		app.ScopeFlag,
		app.NamespacesFlag,
		app.KubeconfigFlag,
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		scope := app.Scope(c)
		namespaces := app.Namespaces(c)

		elevated, err := system.IsElevated()

		if err != nil {
			return err
		}

		if !elevated {
			cli.Fatal("This command must be run as root!")
		}

		return startCatapult(c.Context, client, namespaces, scope)
	},
}

func startCatapult(ctx context.Context, client kubernetes.Client, namespaces []string, scope string) error {
	if scope == "" && len(namespaces) > 0 {
		scope = namespaces[0]
	}

	if scope == "" {
		scope = client.Namespace()
	}

	catapult, err := catapult.New(client, catapult.CatapultOptions{
		Scope:      scope,
		Namespaces: namespaces,

		IncludeIngress: true,
	})

	if err != nil {
		return err
	}

	return catapult.Start(ctx)
}
