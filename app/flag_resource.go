package app

import (
	"context"

	"github.com/adrianliechti/go-cli"
)

var ContainerFlag = &cli.StringFlag{
	Name:  "container",
	Usage: "container",
}

func Container(ctx context.Context, cmd *cli.Command) string {
	return cmd.String(ContainerFlag.Name)
}

var NamespaceFlag = &cli.StringFlag{
	Name:  "namespace",
	Usage: "namespace scope for this request",
}

func Namespace(ctx context.Context, cmd *cli.Command) string {
	return cmd.String(NamespaceFlag.Name)
}

var NamespacesFlag = &cli.StringSliceFlag{
	Name:  "namespace",
	Usage: "namespaces for this request",
}

func Namespaces(ctx context.Context, cmd *cli.Command) []string {
	return cmd.StringSlice(NamespacesFlag.Name)
}

var ScopeFlag = &cli.StringFlag{
	Name:  "scope",
	Usage: "scope",
}

func Scope(ctx context.Context, cmd *cli.Command) string {
	return cmd.String(ScopeFlag.Name)
}
