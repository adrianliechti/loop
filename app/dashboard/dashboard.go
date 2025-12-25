package dashboard

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/system"
	"k8s.io/client-go/rest"
)

var (
	//go:embed public/*
	publicFS embed.FS
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
		config := client.Config()

		port, err := system.FreePort(8888)

		if err != nil {
			return err
		}

		tr, err := rest.TransportFor(config)

		if err != nil {
			return err
		}

		target, path, err := rest.DefaultServerUrlFor(config)

		if err != nil {
			return fmt.Errorf("failed to parse host: %w", err)
		}

		target.Path = path

		fs, _ := fs.Sub(publicFS, "public")

		mux := http.NewServeMux()

		proxy := &httputil.ReverseProxy{
			Transport: tr,

			Rewrite: func(r *httputil.ProxyRequest) {
				r.SetURL(target)
				r.Out.Host = target.Host
			},
		}

		mux.Handle("/api/", proxy)
		mux.Handle("/", http.FileServerFS(fs))

		server := &http.Server{
			Addr:    fmt.Sprintf("localhost:%d", port),
			Handler: mux,
		}

		go func() {
			<-ctx.Done()
			server.Shutdown(context.Background())
		}()

		return server.ListenAndServe()
	},
}
