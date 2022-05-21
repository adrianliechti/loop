package kafka

import (
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
)

const (
	Kafka = "kafka"
)

var Command = &cli.Command{
	Name:  Kafka,
	Usage: "local Kafka server",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		local.ListCommand(Kafka),

		CreateCommand(),
		local.DeleteCommand(Kafka),

		local.LogsCommand(Kafka),
		local.ShellCommand(Kafka, "/bin/bash"),
	},
}
