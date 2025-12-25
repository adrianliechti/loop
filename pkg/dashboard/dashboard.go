package dashboard

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/adrianliechti/loop/pkg/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	//go:embed public/*
	publicFS embed.FS
)

type DashboardOptions struct {
	Port int

	OpenAIKey     string
	OpenAIBaseURL string
}

func Run(ctx context.Context, client kubernetes.Client, options *DashboardOptions) error {
	if options == nil {
		options = new(DashboardOptions)
	}

	if options.Port == 0 {
		options.Port = 8888
	}

	config := client.Config()

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

	if options.OpenAIBaseURL != "" {
		target, err := url.Parse(options.OpenAIBaseURL)

		if err != nil {
			return err
		}

		proxy := &httputil.ReverseProxy{
			Rewrite: func(r *httputil.ProxyRequest) {
				r.Out.URL.Path = strings.TrimPrefix(r.Out.URL.Path, "/openai/v1")

				r.SetURL(target)

				if options.OpenAIKey != "" {
					r.Out.Header.Set("Authorization", "Bearer "+options.OpenAIKey)
				}

				r.Out.Host = target.Host
			},
		}

		mux.Handle("/openai/v1/", proxy)
	}

	mux.Handle("/", http.FileServerFS(fs))

	server := &http.Server{
		Addr:    fmt.Sprintf("localhost:%d", options.Port),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	return server.ListenAndServe()
}
