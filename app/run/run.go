package run

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/remote/run"
)

var Command = &cli.Command{
	Name:  "run",
	Usage: "run cluster container",

	Flags: []cli.Flag{
		app.NamespaceFlag,

		&cli.StringSliceFlag{
			Name:    "port",
			Aliases: []string{"p"},
			Usage:   "forward port locally",
		},

		&cli.StringSliceFlag{
			Name:    "volume",
			Aliases: []string{"v"},
			Usage:   "sync volume remotely",
		},
	},

	Action: func(ctx context.Context, cmd *cli.Command) error {
		client := app.MustClient(ctx, cmd)

		image := cmd.Args().Get(0)
		namespace := app.Namespace(ctx, cmd)

		ports := mustParsePorts(cmd.StringSlice("port"))
		volumes := mustParseVolumes(cmd.StringSlice("volume"))

		container := &run.Container{
			Image: image,

			TTY: true,

			Stdin:  os.Stdin,
			Stdout: os.Stdout,
			Stderr: os.Stderr,

			Ports:   ports,
			Volumes: volumes,
		}

		options := &run.RunOptions{
			Namespace: namespace,

			SyncMode: run.SyncModeMount,
		}

		return run.Run(ctx, client, container, options)
	},
}

func mustParsePorts(ports []string) []run.Port {
	var result []run.Port

	for _, v := range ports {
		parts := strings.Split(v, ":")

		if len(parts) != 2 {
			panic("ports must be in the form of 'source:target'")
		}

		source, _ := strconv.Atoi(parts[0])
		target, _ := strconv.Atoi(parts[1])

		if source <= 0 || target <= 0 {
			panic("invalid port forward")
		}

		result = append(result, run.Port{
			Source: source,
			Target: target,
		})
	}

	return result
}

func mustParseVolumes(volumes []string) []run.Volume {
	var result []run.Volume

	for _, v := range volumes {
		parts := strings.Split(v, ":")

		if len(parts) < 2 {
			panic("volume must be in the form of 'source:target'")
		}

		source := strings.Join(parts[:len(parts)-1], ":")
		target := parts[len(parts)-1]

		source, err := filepath.Abs(source)

		if err != nil {
			source = ""
		}

		if source == "" || target == "" {
			panic("invalid volume mount")
		}

		result = append(result, run.Volume{
			Source: source,
			Target: target,
		})
	}

	return result
}
