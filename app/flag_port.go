package app

import (
	"errors"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/system"
)

var PortFlag = &cli.IntFlag{
	Name:  "port",
	Usage: "port",
}

var PortsFlag = &cli.IntSliceFlag{
	Name:  "port",
	Usage: "port",
}

func Port(c *cli.Context) int {
	return c.Int(PortFlag.Name)
}

func MustPort(c *cli.Context) int {
	port := Port(c)

	if port <= 0 {
		cli.Fatal(errors.New("port missing"))
	}

	return port
}

func Ports(c *cli.Context) []int {
	return c.IntSlice(PortFlag.Name)
}

func MustPorts(c *cli.Context) []int {
	ports := Ports(c)

	if len(ports) == 0 {
		cli.Fatal(errors.New("ports missing"))
	}

	return ports
}

func PortOrRandom(c *cli.Context, preference int) (int, error) {
	port := Port(c)

	if port > 0 {
		return port, nil
	}

	return system.FreePort(preference)
}

func MustPortOrRandom(c *cli.Context, preference int) int {
	port, err := PortOrRandom(c, preference)

	if err != nil {
		cli.Fatal(err)
	}

	return port
}

func RandomPort(c *cli.Context, preference int) (int, error) {
	return system.FreePort(preference)
}

func MustRandomPort(c *cli.Context, preference int) int {
	port, err := RandomPort(c, preference)

	if err != nil {
		cli.Fatal(err)
	}

	return port
}
