package config

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
)

var contextCommand = &cli.Command{
	Name:  "context",
	Usage: "switch context",

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		if c.NArg() > 1 {
			cli.Fatal("invalid context arguments")
		}

		if c.NArg() == 0 {
			return selectContexts(c.Context, client)
		}

		name := c.Args().First()
		return switchContext(c.Context, client, name)
	},
}

func selectContexts(ctx context.Context, client kubernetes.Client) error {
	items, err := listContexts(ctx, client)

	if err != nil {
		return err
	}

	_, name, err := cli.Select("select context", items)

	if err != nil {
		return err
	}

	return switchContext(ctx, client, name)
}

func switchContext(ctx context.Context, client kubernetes.Client, context string) error {
	kubectl := "kubectl"

	cmd := exec.CommandContext(ctx, kubectl, "config", "use-context", context)
	cmd.Env = append(os.Environ(), "KUBECONFIG="+client.ConfigPath())

	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return errors.New(string(exitError.Stderr))
		}

		return err
	}

	return nil
}

func listContexts(ctx context.Context, client kubernetes.Client) ([]string, error) {
	kubectl := "kubectl"

	cmd := exec.CommandContext(ctx, kubectl, "config", "get-contexts", "-o", "name")
	cmd.Env = append(os.Environ(), "KUBECONFIG="+client.ConfigPath())

	output, err := cmd.Output()

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return nil, errors.New(string(exitError.Stderr))
		}

		return nil, err
	}

	text := strings.Trim(string(output), "\r\n")
	contexts := strings.Split(text, "\n")

	return contexts, nil
}
