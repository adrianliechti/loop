package docker

import (
	"context"

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

		if namespace == "" {
			namespace = client.Namespace()
		}

		resourceCPU := resource.MustParse(cmd.String("cpu"))
		resourceMemory := resource.MustParse(cmd.String("memory"))
		storageSize := resource.MustParse(cmd.String("storage"))

		return docker.Create(ctx, client, &docker.CreateOptions{
			Name:      name,
			Namespace: namespace,

			CPU:     resourceCPU,
			Memory:  resourceMemory,
			Storage: storageSize,
		})
	},
}
