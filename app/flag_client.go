package app

import (
	"context"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
)

var KubeconfigFlag = &cli.StringFlag{
	Name:  "kubeconfig",
	Usage: "path to the kubeconfig file",
}

func Client(ctx context.Context, cmd *cli.Command) (kubernetes.Client, error) {
	kubeconfig := cmd.String(KubeconfigFlag.Name)

	return kubernetes.NewFromFile(kubeconfig)
}

func MustClient(ctx context.Context, cmd *cli.Command) kubernetes.Client {
	client, err := Client(ctx, cmd)

	if err != nil {
		cli.Fatal(err)
	}

	return client
}
