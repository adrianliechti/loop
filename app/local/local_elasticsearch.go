package local

import (
	"context"
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var elasticsearchCommand = &cli.Command{
	Name:  "elasticsearch",
	Usage: "local Elasticsearch server",

	Flags: []cli.Flag{
		app.PortFlag,
	},

	Action: func(c *cli.Context) error {
		port := app.MustPortOrRandom(c, 9200)
		return startElasticsearch(c.Context, port)
	},
}

func startElasticsearch(ctx context.Context, port int) error {
	image := "docker.elastic.co/elasticsearch/elasticsearch:7.15.0"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	target := 9200

	if port == 0 {
		port = target
	}

	username := "elastic"
	password := "notsecure"

	cli.Info()

	cli.Table([]string{"Name", "Value"}, [][]string{
		{"Host", fmt.Sprintf("localhost:%d", port)},
		{"Username", username},
		{"Password", password},
		{"URL", fmt.Sprintf("http://%s:%s@localhost:%d", username, password, port)},
	})

	cli.Info()

	options := docker.RunOptions{
		Env: map[string]string{
			"node.name": "es",

			"cluster.name":   "default",
			"discovery.type": "single-node",

			"xpack.security.enabled": "true",

			"ELASTIC_PASSWORD": password,
		},

		Ports: map[int]int{
			port: target,
		},

		// Volumes: map[string]string{
		// 	name: "/usr/share/elasticsearch/data",
		// },
	}

	return docker.RunInteractive(ctx, image, options)
}
