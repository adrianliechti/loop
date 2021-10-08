package docker

import (
	"github.com/adrianliechti/loop/pkg/cli"
)

var Command = &cli.Command{
	Name:  "docker",
	Usage: "manage Docker daemons",

	Subcommands: []*cli.Command{
		browseCommand,
		scanCommand,
		lintCommand,
		analyzeCommand,
	},
}
