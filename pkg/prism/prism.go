package prism

import (
	"github.com/adrianliechti/prism/pkg/config"
	"github.com/adrianliechti/prism/pkg/server"
)

type Server = server.Server

func New() (*Server, error) {
	cfg, err := config.New()

	if err != nil {
		return nil, err
	}

	return server.New(cfg)
}
