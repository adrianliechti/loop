package config

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
)

var importCommand = &cli.Command{
	Name:  "import",
	Usage: "import Kubernetes config",

	Flags: []cli.Flag{
		&cli.PathFlag{
			Name: "filename",
			Aliases: []string{
				"f",
			},
			Required:  true,
			TakesFile: true,
		},
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		path := c.Path("filename")

		return importConfig(c.Context, client, path)
	},
}

func importConfig(ctx context.Context, client kubernetes.Client, path string) error {
	kubectl := "kubectl"

	files := []string{
		path,
	}

	kubeconfig := client.ConfigPath()

	if _, err := os.Stat(kubeconfig); err == nil {
		files = append(files, kubeconfig)
	}

	sep := ":"

	if runtime.GOOS == "windows" {
		sep = ";"
	}

	cmd := exec.CommandContext(ctx, kubectl, "config", "view", "--merge", "--flatten")
	cmd.Env = append(os.Environ(), "KUBECONFIG="+strings.Join(files, sep))

	output, err := cmd.Output()

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return errors.New(string(exitError.Stderr))
		}

		return err
	}

	if err := os.WriteFile(kubeconfig, output, 0600); err != nil {
		return err
	}

	return nil
}
