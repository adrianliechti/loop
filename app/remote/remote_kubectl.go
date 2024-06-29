package remote

import (
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
)

var kubectlCommand = &cli.Command{
	Name:  "kubectl",
	Usage: "run cluster kubectl",

	SkipFlagParsing: true,

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		namespace := app.Namespace(c)

		if namespace == "" {
			namespace = client.Namespace()
		}

		command := append([]string{"kubectl"}, c.Args().Slice()...)

		return runToolKit(c.Context, client, namespace, command)
	},
}
