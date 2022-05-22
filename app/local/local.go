package local

import (
	"context"
	"errors"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

const (
	KindKey = "local.loop.kind"
)

func SelectContainer(ctx context.Context, kind string) (string, error) {
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

func MustContainer(ctx context.Context, kind string) string {
	container, err := SelectContainer(ctx, kind)

	if err != nil {
		cli.Fatal(err)
	}

	return container
}

func ListCommand(kind string) *cli.Command {
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

func DeleteCommand(kind string) *cli.Command {
	return &cli.Command{
		Name:  "delete",
		Usage: "delete instance",

		Action: func(c *cli.Context) error {
			ctx := c.Context
			container := MustContainer(ctx, kind)

			return docker.Delete(ctx, container, docker.DeleteOptions{
				Force:   true,
				Volumes: true,
			})
		},
	}
}

func LogsCommand(kind string) *cli.Command {
	return &cli.Command{
		Name:  "logs",
		Usage: "show instance logs",

		Action: func(c *cli.Context) error {
			ctx := c.Context
			container := MustContainer(ctx, kind)

			return docker.Logs(ctx, container, docker.LogsOptions{
				Follow: true,
			})
		},
	}
}

func ShellCommand(kind, shell string, arg ...string) *cli.Command {
	return &cli.Command{
		Name:  "shell",
		Usage: "run shell in instance (" + shell + ")",

		Action: func(c *cli.Context) error {
			ctx := c.Context
			container := MustContainer(ctx, kind)

			return docker.ExecInteractive(ctx, container, docker.ExecOptions{}, shell, arg...)
		},
	}
}
