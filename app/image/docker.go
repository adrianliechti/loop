package image

import (
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
)

var Command = &cli.Command{
	Name:  "image",
	Usage: "Docker image utilities",

	Category: app.CategoryUtilities,

	Subcommands: []*cli.Command{
		browseCommand,
		scanCommand,
		lintCommand,
		analyzeCommand,
	},
}
