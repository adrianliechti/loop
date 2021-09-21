package connect

import (
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
)

var Command = &cli.Command{
	Name:  "connect",
	Usage: "connect Kubernetes network",

	Flags: []cli.Flag{
		app.NamespaceFlag,
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		namespace := app.NamespaceOrDefault(c)

		return runShuttle(c.Context, client, namespace)
	},
}
