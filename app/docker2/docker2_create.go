package docker2

import (
	"context"
	"fmt"

	"github.com/adrianliechti/go-cli"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/remote/docker"
	"github.com/google/uuid"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var CommandCreate = &cli.Command{
	Name:  "create",
	Usage: "create a new Docker instance",

	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "name",
			Usage: "daemon name",
		},

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

		name := cmd.String("name")
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

		if _, err := client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{}); err == nil {
			return fmt.Errorf("Docker instance '%s' already exists", name)
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
