package redis

import (
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
)

const (
	Redis = "redis"
)

var Command = &cli.Command{
	Name:  Redis,
	Usage: "local Redis server",

	Category: local.CategoryDatabase,

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		local.ListCommand(Redis),

		CreateCommand(),
		local.DeleteCommand(Redis),

		local.LogsCommand(Redis),
		local.ShellCommand(Redis, "/bin/bash"),
		ClientCommand(),
	},
}
