package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/app"
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

		if image == "" {
			return errors.New("image argument is required")
		}

		namespace := app.Namespace(ctx, cmd)

		ports, err := parsePorts(cmd.StringSlice("port"))

		if err != nil {
			return err
		}

		volumes, err := parseVolumes(cmd.StringSlice("volume"))

		if err != nil {
			return err
		}

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

func parsePorts(ports []string) ([]run.Port, error) {
	var result []run.Port

	for _, v := range ports {
		parts := strings.Split(v, ":")

		if len(parts) != 2 {
			return nil, fmt.Errorf("port %q must be in the form 'source:target'", v)
		}

		source, err := strconv.Atoi(parts[0])

		if err != nil || source <= 0 {
			return nil, fmt.Errorf("invalid source port %q", parts[0])
		}

		target, err := strconv.Atoi(parts[1])

		if err != nil || target <= 0 {
			return nil, fmt.Errorf("invalid target port %q", parts[1])
		}

		result = append(result, run.Port{
			Source: source,
			Target: target,
		})
	}

	return result, nil
}

func parseVolumes(volumes []string) ([]run.Volume, error) {
	var result []run.Volume

	for _, v := range volumes {
		parts := strings.Split(v, ":")

		if len(parts) < 2 {
			return nil, fmt.Errorf("volume %q must be in the form 'source:target'", v)
		}

		source := strings.Join(parts[:len(parts)-1], ":")
		target := parts[len(parts)-1]

		if source == "" || target == "" {
			return nil, fmt.Errorf("invalid volume mount %q", v)
		}

		abs, err := filepath.Abs(source)

		if err != nil {
			return nil, fmt.Errorf("invalid volume source %q: %w", source, err)
		}

		result = append(result, run.Volume{
			Source: abs,
			Target: target,
		})
	}

	return result, nil
}
