package etcd

import (
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
)

const (
	ETCD = "etcd"
)

var Command = &cli.Command{
	Name:  "etcd",
	Usage: "local etcd server",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		local.ListCommand(ETCD),

		CreateCommand(),
		local.DeleteCommand(ETCD),

		local.LogsCommand(ETCD),
		local.ShellCommand(ETCD, "/bin/ash"),
	},
}
