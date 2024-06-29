package remote

import (
	"github.com/adrianliechti/loop/pkg/cli"
)

var Command = &cli.Command{
	Name:  "remote",
	Usage: "remote development instances",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		shellCommand,
		dockerCommand,
		codeCommand,
	},
}
