package cluster

import (
	"errors"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kind"
	"github.com/adrianliechti/loop/pkg/minikube"
)

var deleteCommand = &cli.Command{
	Name:  "delete",
	Usage: "delete local cluster",

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
			return kind.Delete(c.Context, name)
		case "minikube":
			return minikube.Delete(c.Context, name)
		default:
			cli.Fatal(errors.New("invalid provider specified"))
		}
		return nil
	},
}
