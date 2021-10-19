package local

import (
	"context"
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var etcdCommand = &cli.Command{
	Name:  "etcd",
	Usage: "local etcd server",

	Flags: []cli.Flag{
		app.PortFlag,
	},

	Action: func(c *cli.Context) error {
		port := app.MustPortOrRandom(c, 2379)
		return startETCD(c.Context, port, 0)
	},
}

func startETCD(ctx context.Context, clientPort, peerPort int) error {
	image := "gcr.io/etcd-development/etcd:v3.3.8"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	targetClientPort := 2379
	targetPeerPort := 2380

	if clientPort == 0 {
		clientPort = targetClientPort
	}

	if peerPort == 0 {
		peerPort = targetPeerPort
	}

	cli.Info()

	cli.Table([]string{"Name", "Value"}, [][]string{
		{"Host", fmt.Sprintf("localhost:%d", clientPort)},
		{"URL", fmt.Sprintf("http://localhost:%d", clientPort)},
	})

	cli.Info()

	options := docker.RunOptions{
		Env: map[string]string{
			"ETCD_NAME": "etcd0",

			"ETCD_DATA_DIR": "/etcd-data",

			"ETCD_LISTEN_PEER_URLS":   fmt.Sprintf("http://0.0.0.0:%d", peerPort),
			"ETCD_LISTEN_CLIENT_URLS": fmt.Sprintf("http://0.0.0.0:%d", clientPort),

			"ETCD_INITIAL_CLUSTER":             fmt.Sprintf("etcd0=http://127.0.0.1:%d", peerPort),
			"ETCD_INITIAL_CLUSTER_STATE":       "new",
			"ETCD_INITIAL_CLUSTER_TOKEN":       "notsecure",
			"ETCD_INITIAL_ADVERTISE_PEER_URLS": fmt.Sprintf("http://127.0.0.1:%d", peerPort),

			"ETCD_ADVERTISE_CLIENT_URLS": fmt.Sprintf("http://127.0.0.1:%d", clientPort),
		},

		Ports: map[int]int{
			clientPort: targetClientPort,
		},

		// Volumes: map[string]string{
		// 	name: /etcd-data",
		// },
	}

	return docker.RunInteractive(ctx, image, options)
}
