package cluster

import (
	"errors"

	"github.com/adrianliechti/loop/app/cluster/extension/dashboard"
	"github.com/adrianliechti/loop/app/cluster/extension/observability"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kind"
	"github.com/adrianliechti/loop/pkg/minikube"
)

var createCommand = &cli.Command{
	Name:  "create",
	Usage: "create local cluster",

	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "name",
			Usage: "cluster name",
		},
		&cli.StringFlag{
			Name:  "provider",
			Usage: "kubernetes provider (kind (default), minikube)",
		},
	},

	Action: func(c *cli.Context) error {
		name := c.String("name")

		provider := c.String("provider")

		switch provider {
		case "kind", "":
			if err := kind.Create(c.Context, name); err != nil {
				return err
			}

			// Cache Images
			for _, image := range append(dashboard.Images, observability.Images...) {
				kind.LoadImage(c.Context, name, image)
			}
		case "minikube":
			if err := minikube.Create(c.Context, name); err != nil {
				return err
			}
		default:
			cli.Fatal(errors.New("invalid provider specified"))
		}

		namespace := "default"
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
