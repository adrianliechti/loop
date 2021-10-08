package remote

import (
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
)

var Command = &cli.Command{
	Name:  "remote",
	Usage: "start remote instances",

	Category: app.CategoryDevelopment,

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		dockerCommand,
		shellCommand,
		codeCommand,
	},
}
