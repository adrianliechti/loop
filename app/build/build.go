package build

import (
	"context"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/remote/build"
)

var Command = &cli.Command{
	Name:  "build",
	Usage: "build image on cluster",

	Flags: []cli.Flag{
		app.NamespaceFlag,

		&cli.StringFlag{
			Name:    "file",
			Usage:   "Name of the Dockerfile (default: \"PATH/Dockerfile\")",
			Aliases: []string{"f"},
		},

		&cli.StringFlag{
			Name:  "image",
			Usage: "image name (format: registry/repository:tag)",
		},

		&cli.BoolFlag{
			Name:  "insecure",
			Usage: "use insecure registry",
		},

		&cli.StringFlag{
			Name:  "username",
			Usage: "registry username",
		},

		&cli.StringFlag{
			Name:  "password",
			Usage: "registry password",
		},
	},

	Action: func(ctx context.Context, cmd *cli.Command) error {
		client := app.MustClient(ctx, cmd)
		namespace := app.Namespace(ctx, cmd)

		file := cmd.String("file")

		path, err := build.ParsePath(cmd.Args().Get(0))

		if err != nil {
			return err
		}

		image, err := build.ParseImage(cmd.String("image"))

		if err != nil {
			return err
		}

		image.Insecure = cmd.Bool("insecure")

		image.Username = cmd.String("username")
		image.Password = cmd.String("password")

		options := &build.RunOptions{
			Namespace: namespace,
		}

		return build.Run(ctx, client, image, path, file, options)
	},
}
