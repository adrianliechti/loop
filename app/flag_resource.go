package app

import (
	"errors"

	"github.com/adrianliechti/loop/pkg/cli"
)

var NameFlag = &cli.StringFlag{
	Name:  "name",
	Usage: "name",
}

func Name(c *cli.Context) string {
	value := c.String(NameFlag.Name)

	if value == "" {
		return ""
	}

	return value
}

func MustName(c *cli.Context) string {
	value := Name(c)

	if value == "" {
		cli.Fatal(errors.New("name missing"))
	}

	return value
}

var ContainerFlag = &cli.StringFlag{
	Name:  "container",
	Usage: "container",
}

func Container(c *cli.Context) string {
	value := c.String(ContainerFlag.Name)

	if value == "" {
		return ""
	}

	return value
}

func ContainerName(c *cli.Context) string {
	value := Container(c)

	if value == "" {
		cli.Fatal(errors.New("container missing"))
	}

	return value
}

var NamespaceFlag = &cli.StringFlag{
	Name:  "namespace",
	Usage: "namespace scope for this request",
}

func Namespace(c *cli.Context) string {
	value := c.String(NamespaceFlag.Name)

	if value == "" {
		return ""
	}

	return value
}

func MustNamespace(c *cli.Context) string {
	value := Namespace(c)

	if value == "" {
		cli.Fatal(errors.New("namespace missing"))
	}

	return value
}

var NamespacesFlag = &cli.StringSliceFlag{
	Name:  "namespace",
	Usage: "namespaces for this request",
}

func Namespaces(c *cli.Context) []string {
	value := c.StringSlice(NamespacesFlag.Name)

	if len(value) == 0 {
		return nil
	}

	return value
}

func MustNamespaces(c *cli.Context) []string {
	value := Namespaces(c)

	if value == nil {
		cli.Fatal(errors.New("namespaces missing"))
	}

	return value
}

var ScopeFlag = &cli.StringFlag{
	Name:  "scope",
	Usage: "scope",
}

func Scope(c *cli.Context) string {
	value := c.String(ScopeFlag.Name)

	if value == "" {
		return ""
	}

	return value
}

func MustScope(c *cli.Context) string {
	value := Scope(c)

	if value == "" {
		cli.Fatal(errors.New("scope missing"))
	}

	return value
}
