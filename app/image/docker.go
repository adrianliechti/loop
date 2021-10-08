package image

import (
	"github.com/adrianliechti/loop/pkg/cli"
)

var Command = &cli.Command{
	Name:  "image",
	Usage: "Docker image utilities",

	Subcommands: []*cli.Command{
		browseCommand,
		scanCommand,
		lintCommand,
		analyzeCommand,
	},
}
