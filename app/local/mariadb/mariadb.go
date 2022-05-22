package mariadb

import (
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
)

const (
	MariaDB = "mariadb"
)

var Command = &cli.Command{
	Name:  MariaDB,
	Usage: "local MariaDB server",

	Category: local.CategoryDatabase,

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		local.ListCommand(MariaDB),

		CreateCommand(),
		local.DeleteCommand(MariaDB),

		local.LogsCommand(MariaDB),
		local.ShellCommand(MariaDB, "/bin/bash"),
		ClientCommand(),
	},
}
