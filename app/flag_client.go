package app

import (
	"context"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
)

var KubeconfigFlag = &cli.StringFlag{
	Name:  "kubeconfig",
	Usage: "path to the kubeconfig file",
}

func Client(ctx context.Context, cmd *cli.Command) (kubernetes.Client, error) {
	return ClientWithContext(ctx, cmd, "")
}

func ClientWithContext(ctx context.Context, cmd *cli.Command, context string) (kubernetes.Client, error) {
	kubeconfig := cmd.String(KubeconfigFlag.Name)

	return kubernetes.NewFromFile(kubeconfig, context)
}

func MustClient(ctx context.Context, cmd *cli.Command) kubernetes.Client {
	return MustClientWithContext(ctx, cmd, "")
}

func MustClientWithContext(ctx context.Context, cmd *cli.Command, context string) kubernetes.Client {
	client, err := ClientWithContext(ctx, cmd, context)

	if err != nil {
		cli.Fatal(err)
	}

	return client
}
