package catapult

import (
	"context"
	"os"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/kubernetes/catapult"
	"github.com/adrianliechti/loop/pkg/system"
	"github.com/adrianliechti/loop/pkg/to"
)

var Command = &cli.Command{
	Name:  "catapult",
	Usage: "connect Kubernetes services",

	Category: app.CategoryCluster,

	Flags: []cli.Flag{
		app.ScopeFlag,
		app.NamespaceFlag,
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		scope := app.Scope(c)
		namespace := app.Namespace(c)

		if namespace == nil {
			namespace = to.StringPtr(client.Namespace())
		}

		if scope == nil {
			scope = namespace
		}

		elevated, err := system.IsElevated()

		if err != nil {
			return err
		}

		if !elevated {
			args := []string{
				os.Args[0],
				"catapult",
			}

			if namespace != nil && len(*namespace) > 0 {
				args = append(args, "--"+app.NamespaceFlag.Name, *namespace)
			}

			if scope != nil && len(*scope) > 0 {
				args = append(args, "--"+app.ScopeFlag.Name, *scope)
			}

			os.Args = args

			if err := system.RunElevated(); err != nil {
				cli.Fatal("This command must be run as root!")
			}

			os.Exit(0)
		}

		return startCatapult(c.Context, client, *namespace, *scope)
	},
}

func startCatapult(ctx context.Context, client kubernetes.Client, namespace, scope string) error {
	return catapult.Start(ctx, client, catapult.CatapultOptions{
		Scope:     scope,
		Namespace: namespace,
	})
}
