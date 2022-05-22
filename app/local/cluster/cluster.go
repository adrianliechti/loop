package cluster

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"path"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kind"
	"github.com/adrianliechti/loop/pkg/kubernetes"
)

const (
	Kubernetes = "cluster"
)

var Command = &cli.Command{
	Name:  Kubernetes,
	Usage: "local Kubernetes cluster",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		ListCommand(),

		CreateCommand(),
		DeleteCommand(),

		DashboardCommand(),
		GrafanaCommand(),
	},
}

func SelectCluster(ctx context.Context) (string, error) {
	list, err := kind.List(ctx)

	var items []string

	if err != nil {
		return "", err
	}

	for _, c := range list {
		items = append(items, c)
	}

	if len(items) == 0 {
		return "", errors.New("no instances found")
	}

	i, _, err := cli.Select("Select instance", items)

	if err != nil {
		return "", err
	}

	return list[i], nil
}

func MustCluster(ctx context.Context) string {
	cluster, err := SelectCluster(ctx)

	if err != nil {
		cli.Fatal(err)
	}

	return cluster
}

func Client(ctx context.Context, name string) (kubernetes.Client, error) {
	dir, err := ioutil.TempDir("", "kind")

	if err != nil {
		return nil, err
	}

	defer os.RemoveAll(dir)

	path := path.Join(dir, "kubeconfig")

	if err := kind.Kubeconfig(ctx, name, path); err != nil {
		return nil, err
	}

	return kubernetes.NewFromConfig(path)
}

func MustClient(ctx context.Context, name string) kubernetes.Client {
	client, err := Client(ctx, name)

	if err != nil {
		cli.Fatal(err)
	}

	return client
}
