package local

import (
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

const (
	ETCD = "etcd"
)

var etcdCommand = &cli.Command{
	Name:  "etcd",
	Usage: "local etcd server",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		listCommand(ETCD),

		createETCD(),
		deleteCommand(ETCD),

		logsCommand(ETCD),
		shellCommand(ETCD, "/bin/ash"),
	},
}

func createETCD() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "create instance",

		Flags: []cli.Flag{
			app.PortFlag,
		},

		Action: func(c *cli.Context) error {
			ctx := c.Context
			image := "gcr.io/etcd-development/etcd:v3.3.8"

			target := 2379
			port := app.MustPortOrRandom(c, target)

			peerPort := 2380

			options := docker.RunOptions{
				Labels: map[string]string{
					KindKey: ETCD,
				},

				Env: map[string]string{
					"ETCD_NAME": "etcd0",

					"ETCD_DATA_DIR": "/etcd-data",

					"ETCD_LISTEN_PEER_URLS":   fmt.Sprintf("http://0.0.0.0:%d", peerPort),
					"ETCD_LISTEN_CLIENT_URLS": fmt.Sprintf("http://0.0.0.0:%d", port),

					"ETCD_INITIAL_CLUSTER":             fmt.Sprintf("etcd0=http://127.0.0.1:%d", peerPort),
					"ETCD_INITIAL_CLUSTER_STATE":       "new",
					"ETCD_INITIAL_CLUSTER_TOKEN":       "notsecure",
					"ETCD_INITIAL_ADVERTISE_PEER_URLS": fmt.Sprintf("http://127.0.0.1:%d", peerPort),

					"ETCD_ADVERTISE_CLIENT_URLS": fmt.Sprintf("http://127.0.0.1:%d", port),
				},

				Ports: map[int]int{
					port: target,
				},

				// Volumes: map[string]string{
				// 	name: /etcd-data",
				// },
			}

			if err := docker.Run(ctx, image, options); err != nil {
				return err
			}

			cli.Table([]string{"Name", "Value"}, [][]string{
				{"Host", fmt.Sprintf("localhost:%d", port)},
				{"URL", fmt.Sprintf("http://localhost:%d", port)},
			})

			return nil
		},
	}
}
