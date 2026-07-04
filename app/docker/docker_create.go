package docker

import (
	"context"
	"fmt"

	"github.com/adrianliechti/go-cli"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/remote/docker"
	"github.com/google/uuid"

	"k8s.io/apimachinery/pkg/api/resource"
)

var CommandCreate = &cli.Command{
	Name:  "create",
	Usage: "create a new Docker instance",

	Flags: []cli.Flag{
		app.NamespaceFlag,

		&cli.StringFlag{
			Name:  "cpu",
			Usage: "cpu resources",
			Value: "500m",
		},

		&cli.StringFlag{
			Name:  "memory",
			Usage: "memory resources",
			Value: "1024Mi",
		},

		&cli.StringFlag{
			Name:  "storage",
			Usage: "storage size for docker data",
			Value: "20Gi",
		},
	},

	Action: func(ctx context.Context, cmd *cli.Command) error {
		client := app.MustClient(ctx, cmd)

		name := cmd.Args().Get(0)
		namespace := app.Namespace(ctx, cmd)

		if name == "" {
			name = cli.MustInput("Name", uuid.NewString()[:5])
		}

		resourceCPU, err := resource.ParseQuantity(cmd.String("cpu"))

		if err != nil {
			return fmt.Errorf("invalid cpu value %q: %w", cmd.String("cpu"), err)
		}

		resourceMemory, err := resource.ParseQuantity(cmd.String("memory"))

		if err != nil {
			return fmt.Errorf("invalid memory value %q: %w", cmd.String("memory"), err)
		}

		storageSize, err := resource.ParseQuantity(cmd.String("storage"))

		if err != nil {
			return fmt.Errorf("invalid storage value %q: %w", cmd.String("storage"), err)
		}

		return docker.Create(ctx, client, &docker.CreateOptions{
			Name:      name,
			Namespace: namespace,

			CPU:     resourceCPU,
			Memory:  resourceMemory,
			Storage: storageSize,
		})
	},
}
