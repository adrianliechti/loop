package local

import (
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

const (
	Kafka = "kafka"
)

var kafkaCommand = &cli.Command{
	Name:  Kafka,
	Usage: "local Kafka server",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		listCommand(Kafka),

		createKafka(),
		deleteCommand(Kafka),

		logsCommand(Kafka),
		shellCommand(Kafka, "/bin/bash"),
	},
}

func createKafka() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "create instance",

		Flags: []cli.Flag{
			app.PortFlag,
		},

		Action: func(c *cli.Context) error {
			ctx := c.Context
			image := "bitnami/kafka:3.2"

			target := 9092
			port := app.MustPortOrRandom(c, target)

			options := docker.RunOptions{
				Labels: map[string]string{
					KindKey: Kafka,
				},

				Env: map[string]string{
					"ALLOW_PLAINTEXT_LISTENER": "yes",

					"KAFKA_CFG_NODE_ID":                        "1",
					"KAFKA_CFG_BROKER_ID":                      "1",
					"KAFKA_CFG_CONTROLLER_QUORUM_VOTERS":       "1@127.0.0.1:9093",
					"KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP": "CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT",
					"KAFKA_CFG_CONTROLLER_LISTENER_NAMES":      "CONTROLLER",
					"KAFKA_CFG_LOG_DIRS":                       "/tmp/logs",
					"KAFKA_CFG_PROCESS_ROLES":                  "broker,controller",
					"KAFKA_CFG_LISTENERS":                      "PLAINTEXT://:9092,CONTROLLER://:9093",

					"KAFKA_CFG_ADVERTISED_LISTENERS": "PLAINTEXT://127.0.0.1:9092",
				},

				Ports: map[int]int{
					port: target,
				},
			}

			if err := docker.Run(ctx, image, options,
				"bash", "-c",
				"/opt/bitnami/scripts/kafka/setup.sh && kafka-storage.sh format --config ${KAFKA_CONF_FILE} --cluster-id default --ignore-formatted && /opt/bitnami/scripts/kafka/run.sh",
			); err != nil {
				return err
			}

			cli.Table([]string{"Name", "Value"}, [][]string{
				{"Host", fmt.Sprintf("localhost:%d", port)},
				{"URL", fmt.Sprintf("kafka://localhost:%d", port)},
			})

			return nil
		},
	}
}
