package vault

import (
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
)

const (
	Vault = "vault"
)

var Command = &cli.Command{
	Name:  Vault,
	Usage: "local Vault server",

	Category: local.CategoryStorage,

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		local.ListCommand(Vault),

		createCommand(),
		local.DeleteCommand(Vault),

		local.LogsCommand(Vault),
		local.ShellCommand(Vault, "/bin/ash"),
	},
}
