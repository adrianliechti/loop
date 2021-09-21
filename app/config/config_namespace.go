package config

import (
	"context"
	"errors"
	"os"
	"os/exec"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var namespaceCommand = &cli.Command{
	Name:  "namespace",
	Usage: "switch default namespace",

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		if c.NArg() > 1 {
			cli.Fatal("invalid namespace arguments")
		}

		if c.NArg() == 0 {
			return selectNamespaces(c.Context, client)
		}

		name := c.Args().First()
		return switchNamespace(c.Context, client, name)
	},
}

func selectNamespaces(ctx context.Context, client kubernetes.Client) error {
	items, err := listNamespace(ctx, client)

	if err != nil {
		return err
	}

	if err != nil {
		return err
	}

	_, name, err := cli.Select("select namespace", items)

	if err != nil {
		return err
	}

	return switchNamespace(ctx, client, name)
}

func switchNamespace(ctx context.Context, client kubernetes.Client, namespace string) error {
	kubectl := "kubectl"

	cmd := exec.CommandContext(ctx, kubectl, "config", "set-context", "--current", "--namespace", namespace)
	cmd.Env = append(os.Environ(), "KUBECONFIG="+client.ConfigPath())

	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return errors.New(string(exitError.Stderr))
		}

		return err
	}

	return nil
}

func listNamespace(ctx context.Context, client kubernetes.Client) ([]string, error) {
	namespaces, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})

	if err != nil {
		return nil, err
	}

	items := make([]string, 0)

	for _, i := range namespaces.Items {
		items = append(items, i.Name)
	}

	return items, nil
}
