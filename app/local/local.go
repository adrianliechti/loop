package local

import (
	"context"
	"errors"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var Command = &cli.Command{
	Name:  "local",
	Usage: "local development instances",

	Category: app.CategoryDevelopment,

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		// mariadbCommand,
		postgresCommand,
		// mongoDBCommand,
		// mssqlCommand,

		// etcdCommand,
		redisCommand,
		// influxdbCommand,
		// elasticsearchCommand,

		// minioCommand,
		vaultCommand,

		natsCommand,
		// rabbitmqCommand,

		// registryCommand,
		// mailtrapCommand,

		// codeCommand,
		// grafanaCommand,
		// jupyterCommand,
	},
}

const (
	KindKey = "local.loop.kind"
)

func selectContainer(ctx context.Context, kind string) (string, error) {
	list, err := docker.List(ctx, docker.ListOptions{
		All: true,

		Filter: []string{
			"label=" + KindKey + "=" + kind,
		},
	})

	var items []string

	if err != nil {
		return "", err
	}

	for _, c := range list {
		name := c.Names[0]
		items = append(items, name)
	}

	if len(items) == 0 {
		return "", errors.New("no instances found")
	}

	i, _, err := cli.Select("Select instance", items)

	if err != nil {
		return "", err
	}

	return list[i].ID, nil
}

func mustContainer(ctx context.Context, kind string) string {
	container, err := selectContainer(ctx, kind)

	if err != nil {
		cli.Fatal(err)
	}

	return container
}

func listCommand(kind string) *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "list instances",

		Action: func(c *cli.Context) error {
			ctx := c.Context

			list, err := docker.List(ctx, docker.ListOptions{
				All: true,

				Filter: []string{
					"label=" + KindKey + "=" + kind,
				},
			})

			if err != nil {
				return err
			}

			for _, c := range list {
				name := c.Names[0]
				cli.Info(name)
			}

			return nil
		},
	}
}

func deleteCommand(kind string) *cli.Command {
	return &cli.Command{
		Name:  "delete",
		Usage: "delete instance",

		Action: func(c *cli.Context) error {
			ctx := c.Context
			container := mustContainer(ctx, kind)

			return docker.Delete(ctx, container, docker.DeleteOptions{
				Force:   true,
				Volumes: true,
			})
		},
	}
}

func logsCommand(kind string) *cli.Command {
	return &cli.Command{
		Name:  "logs",
		Usage: "show instance logs",

		Action: func(c *cli.Context) error {
			ctx := c.Context
			container := mustContainer(ctx, kind)

			return docker.Logs(ctx, container, docker.LogsOptions{
				Follow: true,
			})
		},
	}
}

func shellCommand(kind, shell string, arg ...string) *cli.Command {
	return &cli.Command{
		Name:  "shell",
		Usage: "run shell in instance (" + shell + ")",

		Action: func(c *cli.Context) error {
			ctx := c.Context
			container := mustContainer(ctx, kind)

			return docker.ExecInteractive(ctx, container, docker.ExecOptions{}, shell, arg...)
		},
	}
}
