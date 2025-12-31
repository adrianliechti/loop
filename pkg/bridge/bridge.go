package bridge

import (
	"github.com/adrianliechti/bridge/pkg/config"
	"github.com/adrianliechti/bridge/pkg/server"
	"github.com/adrianliechti/loop/pkg/kubernetes"
)

type Server = server.Server
type PlatformConfig = config.PlatformConfig

func New(client kubernetes.Client, platform *PlatformConfig) (*Server, error) {
	cfg, err := config.New()

	if err != nil {
		return nil, err
	}

	cfg.Platform = platform

	return server.New(cfg)
}
