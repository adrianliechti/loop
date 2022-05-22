package cluster

import (
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
)

func DashboardCommand() *cli.Command {
	return &cli.Command{
		Name:  "dashboard",
		Usage: "open instance Dashboard",

		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "name",
				Usage: "cluster name",
			},
			&cli.StringFlag{
				Name:  app.PortFlag.Name,
				Usage: "local dashboard port",
			},
		},

		Action: func(c *cli.Context) error {
			port := app.MustPortOrRandom(c, 9090)
			name := c.String("name")

			if name == "" {
				name = MustCluster(c.Context)
			}

			client := MustClient(c.Context, name)

			ports := map[int]int{
				port: 9090,
			}

			ready := make(chan struct{})

			go func() {
				<-ready

				url := fmt.Sprintf("http://127.0.0.1:%d", port)
				cli.OpenURL(url)
			}()

			if err := client.ServicePortForward(c.Context, "loop", "dashboard", "", ports, ready); err != nil {
				return err
			}

			return nil
		},
	}
}
