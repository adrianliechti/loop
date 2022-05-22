package cluster

import (
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
	"github.com/adrianliechti/loop/pkg/helm"
	"github.com/adrianliechti/loop/pkg/kind"
	"github.com/adrianliechti/loop/pkg/kubectl"

	"github.com/adrianliechti/loop/app/local/cluster/extension/dashboard"
	"github.com/adrianliechti/loop/app/local/cluster/extension/observability"
)

func CreateCommand() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "create instance",

		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "name",
				Usage: "cluster name",
			},
		},

		Action: func(c *cli.Context) error {
			name := c.String("name")

			if _, _, err := docker.Tool(c.Context); err != nil {
				return err
			}

			if _, _, err := kind.Tool(c.Context); err != nil {
				return err
			}

			if _, _, err := helm.Tool(c.Context); err != nil {
				return err
			}

			if _, _, err := kubectl.Tool(c.Context); err != nil {
				return err
			}

			// if err := kind.Create(c.Context, name); err != nil {
			// 	return err
			// }

			for _, image := range append(dashboard.Images, observability.Images...) {
				docker.Pull(c.Context, image)
				kind.LoadImage(c.Context, name, image)
			}

			namespace := "loop"
			kubeconfig := ""

			if err := dashboard.Install(c.Context, kubeconfig, namespace); err != nil {
				return err
			}

			if err := observability.Install(c.Context, kubeconfig, namespace); err != nil {
				return err
			}

			return nil
		},
	}
}
