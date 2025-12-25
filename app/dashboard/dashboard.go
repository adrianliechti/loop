package dashboard

import (
	"context"
	"os"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/dashboard"
	"github.com/adrianliechti/loop/pkg/system"
)

var Command = &cli.Command{
	Name:  "dashboard",
	Usage: "open Kubernetes dashboard",

	Flags: []cli.Flag{
		app.ScopeFlag,
		app.NamespacesFlag,
	},

	Action: func(ctx context.Context, cmd *cli.Command) error {
		client := app.MustClient(ctx, cmd)

		port, err := system.FreePort(8888)

		if err != nil {
			return err
		}

		openaiKey := os.Getenv("OPENAI_API_KEY")
		openaiURL := os.Getenv("OPENAI_BASE_URL")

		if openaiURL == "" && openaiKey != "" {
			openaiURL = "https://api.openai.com/v1"
		}

		options := &dashboard.DashboardOptions{
			Port: port,

			OpenAIKey:     openaiKey,
			OpenAIBaseURL: openaiURL,
		}

		return dashboard.Run(ctx, client, options)
	},
}
