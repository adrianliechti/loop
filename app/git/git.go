package git

import (
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
)

var Command = &cli.Command{
	Name:  "git",
	Usage: "git repository tools",

	Category: app.CategoryUtilities,

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		leaksCommand,
		blobsCommand,
		purgeCommand,
	},
}
