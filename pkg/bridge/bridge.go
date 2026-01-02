package bridge

import (
	"github.com/adrianliechti/bridge/pkg/config"
	"github.com/adrianliechti/bridge/pkg/server"
	"github.com/adrianliechti/loop/pkg/kubernetes"
)

type Server = server.Server

type PlatformConfig struct {
	TenancyLabels      []string
	PlatformNamespaces []string
}

func New(client kubernetes.Client, platform *PlatformConfig) (*Server, error) {
	cfg, err := config.New()

	if err != nil {
		return nil, err
	}

	if cfg.Kubernetes != nil && platform != nil {
		cfg.Kubernetes.TenancyLabels = platform.TenancyLabels
		cfg.Kubernetes.PlatformNamespaces = platform.PlatformNamespaces
	}

	return server.New(cfg)
}
