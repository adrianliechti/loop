package remote

import (
	"fmt"
	"os"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
)

var streamCommand = &cli.Command{
	Name:  "stream",
	Usage: "connect cluster stream",

	Hidden: true,

	Flags: []cli.Flag{
		app.KubeconfigFlag,
		app.NamespaceFlag,
		app.NameFlag,
		app.ContainerFlag,
		app.PortFlag,
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		namespace := app.MustNamespace(c)
		name := app.MustName(c)

		container := app.ContainerName(c)
		port := app.MustPort(c)

		return client.PodExec(c.Context, namespace, name, container, []string{"nc", "127.0.0.1", fmt.Sprintf("%d", port)}, false, os.Stdin, os.Stdout, os.Stderr)
	},
}
