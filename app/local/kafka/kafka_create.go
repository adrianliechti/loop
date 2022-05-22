package kafka

import (
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

func CreateCommand() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "create instance",

		Flags: []cli.Flag{
			app.PortFlag,
		},

		Action: func(c *cli.Context) error {
			ctx := c.Context
			image := "confluentinc/cp-kafka:7.1.0"

			target := 9092
			port := app.MustPortOrRandom(c, target)

			options := docker.RunOptions{
				Labels: map[string]string{
					local.KindKey: Kafka,
				},

				Platform: "linux/amd64",

				Env: map[string]string{
					"KAFKA_NODE_ID":   "1",
					"KAFKA_BROKER_ID": "1",

					"KAFKA_LISTENERS":                      "PLAINTEXT://0.0.0.0:9092,CONTROLLER://0.0.0.0:9093",
					"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP": "CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT",

					"KAFKA_PROCESS_ROLES": "broker,controller",

					"KAFKA_CONTROLLER_QUORUM_VOTERS":  "1@localhost:9093",
					"KAFKA_CONTROLLER_LISTENER_NAMES": "CONTROLLER",

					"KAFKA_INTER_BROKER_LISTENER_NAME": "PLAINTEXT",

					"KAFKA_ADVERTISED_LISTENERS": fmt.Sprintf("PLAINTEXT://localhost:%d", port),

					"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR":         "1",
					"KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS":         "0",
					"KAFKA_TRANSACTION_STATE_LOG_MIN_ISR":            "1",
					"KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR": "1",
				},

				Ports: map[int]int{
					port: target,
				},
			}

			args := []string{
				"/bin/bash", "-c",
				"sed -i '/KAFKA_ZOOKEEPER_CONNECT/d' /etc/confluent/docker/configure && sed -i '/cub zk-ready/d' /etc/confluent/docker/ensure && echo \"kafka-storage format --ignore-formatted -t $(kafka-storage random-uuid) -c /etc/kafka/kafka.properties\" >> /etc/confluent/docker/ensure && /etc/confluent/docker/run",
			}

			if err := docker.Run(ctx, image, options, args...); err != nil {
				return err
			}

			cli.Table([]string{"Name", "Value"}, [][]string{
				{"Host", fmt.Sprintf("localhost:%d", port)},
				{"URL", fmt.Sprintf("plaintext://localhost:%d", port)},
			})

			return nil
		},
	}
}
