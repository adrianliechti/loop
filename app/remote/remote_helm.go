package remote

import (
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
)

var helmCommand = &cli.Command{
	Name:  "helm",
	Usage: "run cluster helm",

	SkipFlagParsing: true,

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		namespace := app.Namespace(c)

		command := append([]string{"helm"}, c.Args().Slice()...)

		return runToolKit(c.Context, client, namespace, command)
	},
}
