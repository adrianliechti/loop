package nats

import (
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
)

const (
	NATS = "nats"
)

var Command = &cli.Command{
	Name:  NATS,
	Usage: "local NATS server",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		local.ListCommand(NATS),

		CreateCommand(),
		local.DeleteCommand(NATS),

		local.LogsCommand(NATS),
	},
}
