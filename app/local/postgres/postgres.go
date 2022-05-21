package postgres

import (
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
)

const (
	PostgreSQL = "postgres"
)

var Command = &cli.Command{
	Name:  PostgreSQL,
	Usage: "local PostgreSQL server",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		local.ListCommand(PostgreSQL),

		CreateCommand(),
		local.DeleteCommand(PostgreSQL),

		local.LogsCommand(PostgreSQL),
		local.ShellCommand(PostgreSQL, "/bin/bash"),
		ClientCommand(),
	},
}
