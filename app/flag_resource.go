package app

import (
	"context"
	"errors"

	"github.com/adrianliechti/go-cli"
)

var NameFlag = &cli.StringFlag{
	Name:  "name",
	Usage: "name",
}

func Name(ctx context.Context, cmd *cli.Command) string {
	return cmd.String(NameFlag.Name)
}

func MustName(ctx context.Context, cmd *cli.Command) string {
	value := Name(ctx, cmd)

	if value == "" {
		cli.Fatal(errors.New("name missing"))
	}

	return value
}

var ContainerFlag = &cli.StringFlag{
	Name:  "container",
	Usage: "container",
}

func Container(ctx context.Context, cmd *cli.Command) string {
	return cmd.String(ContainerFlag.Name)
}

func ContainerName(ctx context.Context, cmd *cli.Command) string {
	value := Container(ctx, cmd)

	if value == "" {
		cli.Fatal(errors.New("container missing"))
	}

	return value
}

var NamespaceFlag = &cli.StringFlag{
	Name:  "namespace",
	Usage: "namespace scope for this request",
}

func Namespace(ctx context.Context, cmd *cli.Command) string {
	return cmd.String(NamespaceFlag.Name)
}

func MustNamespace(ctx context.Context, cmd *cli.Command) string {
	value := Namespace(ctx, cmd)

	if value == "" {
		cli.Fatal(errors.New("namespace missing"))
	}

	return value
}

var NamespacesFlag = &cli.StringSliceFlag{
	Name:  "namespace",
	Usage: "namespaces for this request",
}

func Namespaces(ctx context.Context, cmd *cli.Command) []string {
	return cmd.StringSlice(NamespacesFlag.Name)
}

func MustNamespaces(ctx context.Context, cmd *cli.Command) []string {
	value := Namespaces(ctx, cmd)

	if len(value) == 0 {
		cli.Fatal(errors.New("namespaces missing"))
	}

	return value
}

var ScopeFlag = &cli.StringFlag{
	Name:  "scope",
	Usage: "scope",
}

func Scope(ctx context.Context, cmd *cli.Command) string {
	return cmd.String(ScopeFlag.Name)
}

func MustScope(ctx context.Context, cmd *cli.Command) string {
	value := Scope(ctx, cmd)

	if value == "" {
		cli.Fatal(errors.New("scope missing"))
	}

	return value
}
