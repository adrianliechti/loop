package tunnel

import (
	"context"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/remote/tunnel"
)

var Command = &cli.Command{
	Name:  "tunnel",
	Usage: "forward local ports to hosts reachable from the cluster",

	ArgsUsage: "host:port [host:port ...]",

	HideHelpCommand: true,

	Flags: []cli.Flag{
		app.NamespaceFlag,

		&cli.StringFlag{
			Name:  "pod",
			Usage: "tunnel through an existing pod (exec) instead of a dedicated jump pod",
		},

		app.ContainerFlag,
	},

	Action: func(ctx context.Context, cmd *cli.Command) error {
		client := app.MustClient(ctx, cmd)

		forwards, err := tunnel.ParseTargets(cmd.Args().Slice())

		if err != nil {
			return err
		}

		return tunnel.Run(ctx, client, &tunnel.RunOptions{
			Namespace: app.Namespace(ctx, cmd),
			Pod:       cmd.String("pod"),
			Container: app.Container(ctx, cmd),

			Forwards: forwards,
		})
	},
}
