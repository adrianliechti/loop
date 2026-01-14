package docker2

import (
	"github.com/adrianliechti/go-cli"
)

var Command = &cli.Command{
	Name:  "docker2",
	Usage: "manage remote Docker instances",

	Commands: []*cli.Command{
		CommandList,
		CommandCreate,
		CommandDelete,
		CommandConnect,
	},
}
