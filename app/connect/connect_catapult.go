package connect

import (
	"context"
	"os"

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
		app.NamespaceFlag,
		app.KubeconfigFlag,
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		scope := app.Scope(c)
		namespace := app.Namespace(c)

		elevated, err := system.IsElevated()

		if err != nil {
			return err
		}

		if !elevated {
			args := []string{
				os.Args[0],
				"connect",
				"catapult",
			}

			if namespace != "" {
				args = append(args, "--"+app.NamespaceFlag.Name, namespace)
			}

			if scope != "" {
				args = append(args, "--"+app.ScopeFlag.Name, scope)
			}

			args = append(args, "--kubeconfig", client.ConfigPath())

			os.Args = args

			if err := system.RunElevated(); err != nil {
				cli.Fatal("This command must be run as root!")
			}

			os.Exit(0)
		}

		return startCatapult(c.Context, client, namespace, scope)
	},
}

func startCatapult(ctx context.Context, client kubernetes.Client, namespace, scope string) error {
	if scope == "" {
		scope = namespace
	}

	if scope == "" {
		scope = client.Namespace()
	}

	catapult, err := catapult.New(client, catapult.CatapultOptions{
		Scope:     scope,
		Namespace: namespace,
	})

	if err != nil {
		return err
	}

	return catapult.Start(ctx)
}
