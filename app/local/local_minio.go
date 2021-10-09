package local

import (
	"context"
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var minioCommand = &cli.Command{
	Name:  "minio",
	Usage: "local MinIO server",

	Flags: []cli.Flag{
		app.PortFlag,
	},

	Action: func(c *cli.Context) error {
		apiPort := app.MustRandomPort(c, 9000)
		consolePort := app.MustRandomPort(c, apiPort+1)

		return startMinIO(c.Context, apiPort, consolePort)
	},
}

func startMinIO(ctx context.Context, apiPort, consolePort int) error {
	image := "minio/minio"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	apiTarget := 9000
	consoleTarget := 9001

	if apiPort == 0 {
		apiPort = apiTarget
	}

	if consolePort == 0 {
		consolePort = consoleTarget
	}

	username := "root"
	password := "notsecure"

	cli.Info()

	cli.Table([]string{"Name", "Value"}, [][]string{
		{"Host", fmt.Sprintf("localhost:%d", apiPort)},
		{"Username", username},
		{"Password", password},
		{"API", fmt.Sprintf("http://localhost:%d", apiPort)},
		{"Console", fmt.Sprintf("http://localhost:%d", consolePort)},
	})

	cli.Info()

	options := docker.RunOptions{
		Env: map[string]string{
			"MINIO_ROOT_USER":     username,
			"MINIO_ROOT_PASSWORD": password,
		},

		Ports: map[int]int{
			apiPort:     apiTarget,
			consolePort: consoleTarget,
		},

		// Volumes: map[string]string{
		// 	path: "/data",
		// },
	}

	return docker.RunInteractive(ctx, image, options, "server", "/data", "--console-address", ":9001")
}
