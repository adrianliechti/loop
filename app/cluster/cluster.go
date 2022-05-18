package cluster

import (
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
)

var Command = &cli.Command{
	Name:  "cluster",
	Usage: "manage cluster",

	HideHelpCommand: true,

	Category: app.CategoryCluster,

	Subcommands: []*cli.Command{
		createCommand,
		deleteCommand,
	},
}
