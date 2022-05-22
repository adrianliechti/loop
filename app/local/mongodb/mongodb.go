package mongodb

import (
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
)

const (
	MongoDB = "mongodb"
)

var Command = &cli.Command{
	Name:  MongoDB,
	Usage: "local MongoDB server",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		local.ListCommand(MongoDB),

		CreateCommand(),
		local.DeleteCommand(MongoDB),

		local.LogsCommand(MongoDB),
		local.ShellCommand(MongoDB, "/bin/bash"),
		ClientCommand(),
	},
}
