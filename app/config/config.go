package config

import (
	"github.com/adrianliechti/loop/pkg/cli"
)

var Command = &cli.Command{
	Name:  "config",
	Usage: "manage Kubernetes config",

	Subcommands: []*cli.Command{
		importCommand,
		contextCommand,
		namespaceCommand,
	},
}
