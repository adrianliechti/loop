package minio

import (
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
)

const (
	MinIO = "minio"
)

var Command = &cli.Command{
	Name:  MinIO,
	Usage: "local MinIO server",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		local.ListCommand(MinIO),

		CreateCommand(),
		local.DeleteCommand(MinIO),

		local.LogsCommand(MinIO),
		local.ShellCommand(MinIO, "/bin/ash"),
	},
}
