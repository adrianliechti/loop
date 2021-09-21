package app

import (
	"errors"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/system"
)

var PortFlag = &cli.StringFlag{
	Name:  "port",
	Usage: "port",
}

var PortsFlag = &cli.StringSliceFlag{
	Name:  "ports",
	Usage: "ports",
}

func Port(c *cli.Context) string {
	return c.String(PortFlag.Name)
}

func MustPort(c *cli.Context) string {
	port := Port(c)

	if port == "" {
		cli.Fatal(errors.New("port missing"))
	}

	return port
}

func Ports(c *cli.Context) []string {
	return c.StringSlice(PortFlag.Name)
}

func MustPorts(c *cli.Context) []string {
	ports := Ports(c)

	if len(ports) == 0 {
		cli.Fatal(errors.New("ports missing"))
	}

	return ports
}

func PortOrRandom(c *cli.Context, preference string) (string, error) {
	port := Port(c)

	if port != "" {
		return port, nil
	}

	return system.FreePort(preference)
}

func MustPortOrRandom(c *cli.Context, preference string) string {
	port, err := PortOrRandom(c, preference)

	if err != nil {
		cli.Fatal(err)
	}

	return port
}

func RandomPort(c *cli.Context, preference string) (string, error) {
	return system.FreePort(preference)
}

func MustRandomPort(c *cli.Context, preference string) string {
	port, err := RandomPort(c, preference)

	if err != nil {
		cli.Fatal(err)
	}

	return port
}
