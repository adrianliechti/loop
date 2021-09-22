package expose

import (
	"github.com/adrianliechti/loop/pkg/cli"
)

var Command = &cli.Command{
	Name:  "expose",
	Usage: "expose local servers",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		tcpCommand,
	},
}
