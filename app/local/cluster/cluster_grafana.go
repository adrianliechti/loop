package cluster

import (
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
)

func GrafanaCommand() *cli.Command {
	return &cli.Command{
		Name:  "grafana",
		Usage: "open instance Grafana",

		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  app.PortFlag.Name,
				Usage: "local dashboard port",
			},
		},

		Action: func(c *cli.Context) error {
			port := app.MustPortOrRandom(c, 3000)
			name := c.String("name")

			if name == "" {
				name = MustCluster(c.Context)
			}

			client := MustClient(c.Context, name)

			ports := map[int]int{
				port: 3000,
			}

			ready := make(chan struct{})

			go func() {
				<-ready

				url := fmt.Sprintf("http://127.0.0.1:%d", port)
				cli.OpenURL(url)
			}()

			if err := client.ServicePortForward(c.Context, "loop", "grafana", "", ports, ready); err != nil {
				return err
			}

			return nil
		},
	}
}
