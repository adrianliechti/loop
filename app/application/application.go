package application

import (
	"github.com/adrianliechti/loop/pkg/cli"
)

var Command = &cli.Command{
	Name:  "app",
	Usage: "manage Kubernetes apps",

	Subcommands: []*cli.Command{
		listCommand,
		infoCommand,
		logCommand,
		clocCommand,
		packCommand,
	},
}
