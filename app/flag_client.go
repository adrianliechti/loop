package app

import (
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
)

var KubeconfigFlag = &cli.StringFlag{
	Name:  "kubeconfig",
	Usage: "path to the kubeconfig file",
}

func Client(c *cli.Context) (kubernetes.Client, error) {
	kubeconfig := c.String(KubeconfigFlag.Name)

	return kubernetes.NewFromFile(kubeconfig)
}

func MustClient(c *cli.Context) kubernetes.Client {
	client, err := Client(c)

	if err != nil {
		cli.Fatal(err)
	}

	return client
}
