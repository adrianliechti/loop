package bridge

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/bridge"
	"github.com/adrianliechti/loop/pkg/system"
)

var Command = &cli.Command{
	Name:  "bridge",
	Usage: "open Bridge",

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

		openaiModel := os.Getenv("OPENAI_MODEL")

		if openaiURL == "" && openaiKey != "" {
			openaiURL = "https://api.openai.com/v1"

			if openaiModel == "" {
				openaiModel = "gpt-5.2"
			}
		}

		options := &bridge.Options{
			OpenAIKey:     openaiKey,
			OpenAIModel:   openaiModel,
			OpenAIBaseURL: openaiURL,
		}

		server, err := bridge.New(client, options)

		if err != nil {
			return err
		}

		addr := fmt.Sprintf("localhost:%d", port)

		time.AfterFunc(1*time.Second, func() {
			cli.OpenURL(fmt.Sprintf("http://localhost:%d", port))
		})

		return server.ListenAndServe(ctx, addr)
	},
}
