package mssql

import (
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
)

const (
	MSSQL = "mssql"
)

var Command = &cli.Command{
	Name:  MSSQL,
	Usage: "local MSSQL server",

	Category: local.CategoryDatabase,

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		local.ListCommand(MSSQL),

		CreateCommand(),
		local.DeleteCommand(MSSQL),

		local.LogsCommand(MSSQL),
		local.ShellCommand(MSSQL, "/bin/bash"),
		ClientCommand(),
	},
}
