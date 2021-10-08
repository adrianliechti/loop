package config

import (
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
)

var Command = &cli.Command{
	Name:  "config",
	Usage: "manage Kubernetes config",

	Category: app.CategoryCluster,

	Subcommands: []*cli.Command{
		importCommand,
		contextCommand,
		namespaceCommand,
	},
}
