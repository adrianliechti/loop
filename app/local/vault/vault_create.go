package vault

import (
	"fmt"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/app/local"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"

	"github.com/sethvargo/go-password/password"
)

func createCommand() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "create instance",

		Flags: []cli.Flag{
			app.PortFlag,
		},

		Action: func(c *cli.Context) error {
			ctx := c.Context
			image := "vault:latest"

			target := 8200
			port := app.MustPortOrRandom(c, target)

			token, err := password.Generate(10, 4, 0, false, false)

			if err != nil {
				return err
			}

			options := docker.RunOptions{
				Labels: map[string]string{
					local.KindKey: Vault,
				},

				Env: map[string]string{
					"VAULT_DEV_ROOT_TOKEN_ID": token,
				},

				Ports: map[int]int{
					port: target,
				},
			}

			if err := docker.Run(ctx, image, options); err != nil {
				return err
			}

			cli.Table([]string{"Name", "Value"}, [][]string{
				{"Host", fmt.Sprintf("localhost:%d", port)},
				{"Token", token},
				{"URL", fmt.Sprintf("http://localhost:%d", port)},
			})

			return nil
		},
	}
}
