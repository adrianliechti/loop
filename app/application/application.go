package application

import (
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
)

var Command = &cli.Command{
	Name:  "app",
	Usage: "manage Kubernetes apps",

	HideHelpCommand: true,

	Category: app.CategoryCluster,

	Subcommands: []*cli.Command{
		listCommand,
		infoCommand,
		logCommand,
	},
}
