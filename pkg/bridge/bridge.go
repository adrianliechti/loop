package bridge

import (
	"github.com/adrianliechti/bridge/pkg/server"
	"github.com/adrianliechti/loop/pkg/kubernetes"
)

type Server = server.Server
type Options = server.Options

func New(client kubernetes.Client, options *Options) (*Server, error) {
	config := client.Config()
	return server.New(config, options)
}
