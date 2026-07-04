package granite

import (
	"github.com/adrianliechti/granite/pkg/config"
	"github.com/adrianliechti/granite/pkg/server"
)

type Server = server.Server

func New() (*Server, error) {
	cfg, err := config.New()

	if err != nil {
		return nil, err
	}

	return server.New(cfg)
}
