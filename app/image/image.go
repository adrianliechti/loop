package image

import (
	"errors"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
)

var Command = &cli.Command{
	Name:  "image",
	Usage: "image utilities & analyzers",

	HideHelpCommand: true,

	Category: app.CategoryUtilities,

	Subcommands: []*cli.Command{
		packCommand,
		browseCommand,
		bomCommand,
		scanCommand,
		lintCommand,
		analyzeCommand,
	},
}

var ImageFlag = &cli.StringFlag{
	Name:     "image",
	Usage:    "docker image",
	Required: true,
}

func Image(c *cli.Context) string {
	image := c.String(ImageFlag.Name)
	return image
}

func MustImage(c *cli.Context) string {
	image := Image(c)

	if len(image) == 0 {
		cli.Fatal(errors.New("image missing"))
	}

	return image
}
