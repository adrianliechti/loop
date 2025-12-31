package prism

import (
	"context"
	"fmt"
	"time"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/pkg/prism"
	"github.com/adrianliechti/loop/pkg/system"
)

var Command = &cli.Command{
	Name:  "prism",
	Usage: "run Prism API client",

	Action: func(ctx context.Context, cmd *cli.Command) error {
		port, err := system.FreePort(9999)

		if err != nil {
			return err
		}

		srv, err := prism.New()

		if err != nil {
			return err
		}

		url := fmt.Sprintf("http://localhost:%d", port)
		addr := fmt.Sprintf("localhost:%d", port)

		time.AfterFunc(500*time.Millisecond, func() {
			cli.Infof("Prism on %s", url)
			cli.OpenURL(url)
		})

		return srv.ListenAndServe(ctx, addr)
	},
}
