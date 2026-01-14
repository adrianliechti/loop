package docker

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/remote/docker"
)

var CommandConnect = &cli.Command{
	Name:  "connect",
	Usage: "connect to an existing Docker instance",

	Flags: []cli.Flag{
		app.NamespaceFlag,
		app.NameFlag,

		&cli.StringSliceFlag{
			Name:    "port",
			Aliases: []string{"p"},
			Usage:   "forward port locally (e.g., 8080:80)",
		},

		&cli.StringSliceFlag{
			Name:    "volume",
			Aliases: []string{"v"},
			Usage:   "mount volume remotely (e.g., ./data:/data)",
		},
	},

	Action: func(ctx context.Context, cmd *cli.Command) error {
		client := app.MustClient(ctx, cmd)

		name := cmd.Args().Get(0)
		namespace := app.Namespace(ctx, cmd)

		if namespace == "" {
			namespace = client.Namespace()
		}

		ports := mustParsePorts(cmd.StringSlice("port"))
		volumes := mustParseVolumes(cmd.StringSlice("volume"))

		if name == "" {
			candidates, err := docker.List(ctx, client, &docker.ListOptions{
				Namespace: namespace,
			})

			if err != nil {
				return err
			}

			if len(candidates) == 0 {
				return errors.New("no Docker instances found")
			}

			var items []string

			for _, c := range candidates {
				items = append(items, c.Name)
			}

			_, name = cli.MustSelect("Daemon", items)
		}

		options := &docker.ConnectOptions{
			Namespace: namespace,

			Ports:   ports,
			Volumes: volumes,
		}

		if len(volumes) > 0 {
			options.SyncMode = docker.SyncModeMount
		}

		if err := docker.Connect(ctx, client, name, options); err != nil {
			return err
		}

		return nil
	},
}

func mustParsePorts(ports []string) []docker.Port {
	var result []docker.Port

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

		result = append(result, docker.Port{
			Source: source,
			Target: target,
		})
	}

	return result
}

func mustParseVolumes(volumes []string) []docker.Volume {
	var result []docker.Volume

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

		result = append(result, docker.Volume{
			Source: source,
			Target: target,
		})
	}

	return result
}
