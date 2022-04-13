package local

import (
	"context"
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var rabbitmqCommand = &cli.Command{
	Name:  "rabbitmq",
	Usage: "local RabbitMQ server",

	Flags: []cli.Flag{
		//app.PortFlag,
	},

	Action: func(c *cli.Context) error {
		amqpPort := app.MustRandomPort(c, 5672)
		httpPort := app.MustRandomPort(c, 15672)

		return startRabbitMQ(c.Context, amqpPort, httpPort)
	},
}

func startRabbitMQ(ctx context.Context, amqpPort, httpPort int) error {
	image := "rabbitmq:3-management"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	amqpTarget := 5672
	httpTarget := 15672

	if amqpPort == 0 {
		amqpPort = amqpTarget
	}

	if httpPort == 0 {
		httpPort = httpTarget
	}

	username := "admin"
	password := "notsecure"

	cli.Info()

	cli.Table([]string{"Name", "Value"}, [][]string{
		{"Host", fmt.Sprintf("localhost:%d", amqpPort)},
		{"Username", username},
		{"Password", password},
		{"URL", fmt.Sprintf("http://localhost:%d", httpPort)},
	})

	cli.Info()

	options := docker.RunOptions{
		Env: map[string]string{
			"RABBITMQ_DEFAULT_USER": username,
			"RABBITMQ_DEFAULT_PASS": password,
		},

		Ports: map[int]int{
			amqpPort: amqpTarget,
			httpPort: httpTarget,
		},
	}

	return docker.RunInteractive(ctx, image, options)
}
