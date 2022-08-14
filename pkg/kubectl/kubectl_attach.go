package kubectl

import (
	"context"
	"os"
	"os/exec"
)

func Attach(ctx context.Context, kubeconfig, namespace, name, container string) error {
	tool, _, err := Info(ctx)

	if err != nil {
		return err
	}

	args := []string{
		"--kubeconfig",
		kubeconfig,
		"attach",
		"-it",
		"-n",
		namespace,
		name,
	}

	if container != "" {
		args = append(args, "-c", container)
	}

	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
