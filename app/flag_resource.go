package app

import (
	"errors"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/to"
)

var NameFlag = &cli.StringFlag{
	Name:  "name",
	Usage: "name",
}

func Name(c *cli.Context) *string {
	value := c.String(NameFlag.Name)

	if value == "" {
		return nil
	}

	return to.StringPtr(value)
}

func MustName(c *cli.Context) string {
	value := to.String(Name(c))

	if value == "" {
		cli.Fatal(errors.New("name missing"))
	}

	return value
}

var NamespaceFlag = &cli.StringFlag{
	Name:  "namespace",
	Usage: "namespace scope for this request",
}

func Namespace(c *cli.Context) *string {
	value := c.String(NamespaceFlag.Name)

	if value == "" {
		return nil
	}

	return to.StringPtr(value)
}

func MustNamespace(c *cli.Context) string {
	value := to.String(Namespace(c))

	if value == "" {
		cli.Fatal(errors.New("namespace missing"))
	}

	return value
}

var ScopeFlag = &cli.StringFlag{
	Name:  "scope",
	Usage: "scope",
}

func Scope(c *cli.Context) *string {
	value := c.String(ScopeFlag.Name)

	if value == "" {
		return nil
	}

	return to.StringPtr(value)
}

func MustScope(c *cli.Context) string {
	value := to.String(Scope(c))

	if value == "" {
		cli.Fatal(errors.New("scope missing"))
	}

	return value
}
