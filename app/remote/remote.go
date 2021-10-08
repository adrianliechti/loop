package remote

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
	"github.com/adrianliechti/loop/pkg/kubectl"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/ssh"
)

var Command = &cli.Command{
	Name:  "remote",
	Usage: "remote instances",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		dockerCommand,
		shellCommand,
		codeCommand,
	},
}

func runTunnel(ctx context.Context, client kubernetes.Client, namespace, name string, port int, tunnels map[int]int) error {
	ssh, _, err := ssh.Tool(ctx)

	if err != nil {
		return err
	}

	kubectl, _, err := kubectl.Tool(ctx)

	if err != nil {
		return err
	}

	args := []string{
		"-q",
		"-t",
		"-l",
		"root",
		"-o",
		"UserKnownHostsFile=/dev/null",
		"-o",
		"StrictHostKeyChecking=no",
		"-o",
		fmt.Sprintf("ProxyCommand=%s --kubeconfig %s exec -i -n %s %s -c ssh -- nc 127.0.0.1 22", kubectl, client.ConfigPath(), namespace, name),
		"localhost",
	}

	command := "mkdir /mnt/src && sshfs -o allow_other -p 2222 root@localhost:/src /mnt/src && exec /bin/ash"

	if port != 0 {
		args = append(args, "-R", fmt.Sprintf("2222:127.0.0.1:%d", port))
	}

	for source, target := range tunnels {
		args = append(args, "-L", fmt.Sprintf("%d:127.0.0.1:%d", source, target))
	}

	if command != "" {
		args = append(args, command)
	}

	cmd := exec.CommandContext(ctx, ssh, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return cmd.Run()
}

func startServer(ctx context.Context, path string, port int) (string, error) {
	tool, _, err := docker.Tool(ctx)

	if err != nil {
		return "", err
	}

	args := []string{
		"run",
		"-d",

		"--pull",
		"always",

		"--publish",
		fmt.Sprintf("127.0.0.1:%d:22", port),

		"--volume",
		path + ":/src",

		"adrianliechti/loop-tunnel",
	}

	cmd := exec.CommandContext(ctx, tool, args...)

	output, err := cmd.CombinedOutput()

	if err != nil {
		return "", errors.New(string(output))
	}

	text := string(output)
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.TrimRight(text, "\n")

	lines := strings.Split(text, "\n")

	if len(lines) == 0 {
		return "", errors.New("unable to get container id")
	}

	container := lines[len(lines)-1]
	return container, nil
}

func stopServer(ctx context.Context, container string) error {
	tool, _, err := docker.Tool(ctx)

	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, tool, "rm", "--force", container)

	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.New(string(output))
	}

	return err
}
