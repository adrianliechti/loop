package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
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
	OpenAIModel   string
	OpenAIBaseURL string

	PlatformNamespaces  []string
	PlatformSpaceLabels []string
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

		ErrorLog: log.New(io.Discard, "", 0),

		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.Host = target.Host
		},
	}

	mux.Handle("/api/", proxy)
	mux.Handle("/apis/", proxy)

	if options.OpenAIBaseURL != "" {
		target, err := url.Parse(options.OpenAIBaseURL)

		if err != nil {
			return err
		}

		proxy := &httputil.ReverseProxy{
			ErrorLog: log.New(io.Discard, "", 0),

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

	mux.HandleFunc("GET /config.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		config := map[string]any{}

		if options.OpenAIBaseURL != "" {
			ai := map[string]any{}

			if options.OpenAIModel != "" {
				ai["model"] = options.OpenAIModel
			}

			config["ai"] = ai
		}

		if len(options.PlatformNamespaces) > 0 || len(options.PlatformSpaceLabels) > 0 {
			platform := map[string]any{}

			if len(options.PlatformNamespaces) > 0 {
				platform["namespaces"] = options.PlatformNamespaces
			}

			if len(options.PlatformSpaceLabels) > 0 {
				spaces := map[string]any{
					"labels": options.PlatformSpaceLabels,
				}

				platform["spaces"] = spaces
			}

			config["platform"] = platform
		}

		json.NewEncoder(w).Encode(config)
	})

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
