package elasticsearch

import (
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
)

const (
	Elasticsearch = "elasticsearch"
)

var Command = &cli.Command{
	Name:  Elasticsearch,
	Usage: "local Elasticsearch server",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		local.ListCommand(Elasticsearch),

		CreateCommand(),
		local.DeleteCommand(Elasticsearch),

		local.LogsCommand(Elasticsearch),
		local.ShellCommand(Elasticsearch, "/bin/bash"),
	},
}
