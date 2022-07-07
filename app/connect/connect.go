package connect

import (
	"github.com/adrianliechti/loop/pkg/cli"
)

var Command = &cli.Command{
	Name:  "connect",
	Usage: "connect Kubernetes network",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		proxyCommand,
		sshuttleCommand,
		catapultCommand,
	},

	Action: func(c *cli.Context) error {
		return catapultCommand.Action(c)
	},
}
