package kubectl

import (
	"context"
	"os"
	"os/exec"
)

func Exec(ctx context.Context, kubeconfig, namespace, name, container string, path string, arg ...string) error {
	tool, _, err := Info(ctx)

	if err != nil {
		return err
	}

	args := []string{
		"--kubeconfig",
		kubeconfig,
		"exec",
		"-it",
		"-n",
		namespace,
		name,
	}

	if container != "" {
		args = append(args, "-c", container)
	}

	args = append(args, "--")
	args = append(args, path)
	args = append(args, arg...)

	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
