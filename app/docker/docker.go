package docker

import (
	"github.com/adrianliechti/go-cli"
)

var Command = &cli.Command{
	Name:  "docker",
	Usage: "manage remote Docker instances",

	Commands: []*cli.Command{
		CommandList,
		CommandCreate,
		CommandDelete,
		CommandConnect,
	},
}
