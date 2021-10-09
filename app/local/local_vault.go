package local

import (
	"context"
	"fmt"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
)

var vaultCommand = &cli.Command{
	Name:  "vault",
	Usage: "local Vault server",

	Flags: []cli.Flag{},

	Action: func(c *cli.Context) error {
		return startVault(c.Context, 0)
	},
}

func startVault(ctx context.Context, port int) error {
	image := "vault:latest"

	if err := docker.Pull(ctx, image); err != nil {
		return err
	}

	target := 8200

	if port == 0 {
		port = target
	}

	token := "notsecure"

	cli.Info()

	cli.Table([]string{"Name", "Value"}, [][]string{
		{"Host", fmt.Sprintf("localhost:%d", port)},
		{"Token", token},
		{"URL", fmt.Sprintf("http://localhost:%d", port)},
	})

	cli.Info()

	options := docker.RunOptions{
		Env: map[string]string{
			"VAULT_DEV_ROOT_TOKEN_ID": token,
		},

		Ports: map[int]int{
			port: target,
		},
	}

	return docker.RunInteractive(ctx, image, options)
}
