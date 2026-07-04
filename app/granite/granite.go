package granite

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/pkg/granite"
	"github.com/adrianliechti/loop/pkg/system"
)

var Command = &cli.Command{
	Name:  "granite",
	Usage: "run Granite API client",

	Action: func(ctx context.Context, cmd *cli.Command) error {
		port, err := system.FreePort(7777)

		if err != nil {
			return err
		}

		srv, err := granite.New()

		if err != nil {
			return err
		}

		url := fmt.Sprintf("http://localhost:%d", port)
		addr := fmt.Sprintf("localhost:%d", port)

		// Bind before announcing the URL so the browser never opens against a
		// port that failed to bind (or was claimed since FreePort probed it).
		l, err := net.Listen("tcp", addr)

		if err != nil {
			return err
		}

		time.AfterFunc(500*time.Millisecond, func() {
			cli.Infof("Granite on %s", url)
			cli.OpenURL(url)
		})

		server := &http.Server{Handler: srv}

		go func() {
			<-ctx.Done()
			server.Close()
		}()

		if err := server.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}

		return nil
	},
}
