package tool

import (
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
)

var Command = &cli.Command{
	Name:  "tool",
	Usage: "handy tools",

	Category: app.CategoryUtilities,

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		clocCommand,
		swaggerCommand,
	},
}
