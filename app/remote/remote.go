package remote

import (
	"github.com/adrianliechti/loop/pkg/cli"
)

var Command = &cli.Command{
	Name:  "remote",
	Usage: "remote instances",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		dockerCommand,
		shellCommand,
		codeCommand,
	},
}
