package docker

import (
	"context"
	"errors"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/remote/docker"
)

var Command = &cli.Command{
	Name:  "docker",
	Usage: "manage remote Docker instances",

	Commands: []*cli.Command{
		CommandList,
		CommandCreate,
		CommandDelete,
		CommandConnect,
	},
}

// selectInstance prompts for one of the existing daemons in the namespace.
func selectInstance(ctx context.Context, client kubernetes.Client, namespace string) (string, error) {
	candidates, err := docker.List(ctx, client, &docker.ListOptions{
		Namespace: namespace,
	})

	if err != nil {
		return "", err
	}

	if len(candidates) == 0 {
		return "", errors.New("no Docker instances found")
	}

	var items []string

	for _, c := range candidates {
		items = append(items, c.Name)
	}

	_, name := cli.MustSelect("Daemon", items)
	return name, nil
}
