package code

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/remote/run"
	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
)

var Command = &cli.Command{
	Name:  "code",
	Usage: "run cluster VS Code",

	Flags: []cli.Flag{
		app.NamespaceFlag,

		&cli.StringFlag{
			Name:  "stack",
			Usage: "language stack",
		},

		app.PortsFlag,
	},

	Action: func(ctx context.Context, cmd *cli.Command) error {
		client := app.MustClient(ctx, cmd)

		path, err := os.Getwd()

		if err != nil {
			return err
		}

		stacks := []string{
			"default",
			"golang",
			"python",
			"java",
			"dotnet",
			"azure",
		}

		stack := cmd.String("stack")

		if stack == "" {
			i, _, err := cli.Select("select stack", stacks)

			if err != nil {
				return err
			}

			stack = stacks[i]
		}

		if stack == "latest" || stack == "default" {
			stack = ""
		}

		image := "ghcr.io/adrianliechti/loop-code"

		if stack != "" {
			image += ":" + strings.ToLower(stack)
		}

		port := app.MustPortOrRandom(ctx, cmd, 8888)
		namespace := app.Namespace(ctx, cmd)

		if namespace == "" {
			namespace = client.Namespace()
		}

		ports, _ := app.Ports(ctx, cmd)

		if ports == nil {
			ports = map[int]int{}
		}

		ports[port] = 3000

		var runPorts []run.Port

		for s, t := range ports {
			runPorts = append(runPorts, run.Port{
				Source: s,
				Target: t,
			})
		}

		var runVolumes []run.Volume

		runVolumes = append(runVolumes, run.Volume{
			Source: path,
			Target: "/src",

			Identity: &run.Identity{
				UID: 1000,
				GID: 1000,
			},
		})

		container := &run.Container{
			Image: image,

			Stdout: os.Stdout,
			Stderr: os.Stderr,

			Ports:   runPorts,
			Volumes: runVolumes,
		}

		options := &run.RunOptions{
			Name:      "loop-code-" + uuid.NewString()[0:7],
			Namespace: namespace,

			SyncMode: run.SyncModeMount,
		}

		options.OnReady = func(ctx context.Context, client kubernetes.Client, pod *corev1.Pod) error {
			cli.OpenURL(fmt.Sprintf("http://localhost:%d", port))

			return nil
		}

		return run.Run(ctx, client, container, options)
	},
}
