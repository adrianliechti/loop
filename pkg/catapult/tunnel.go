package catapult

import (
	"context"
	"log/slog"

	"github.com/adrianliechti/loop/pkg/kubernetes"
)

type tunnel struct {
	client    kubernetes.Client
	name      string
	namespace string

	hosts []string

	address string
	ports   map[int]int

	cancel context.CancelFunc
}

func newTunnel(client kubernetes.Client, namespace, name, address string, ports map[int]int, hosts []string) *tunnel {
	return &tunnel{
		client: client,

		name:      name,
		namespace: namespace,

		address: address,
		ports:   ports,

		hosts: hosts,
	}
}

func (t *tunnel) Start(ctx context.Context, readyChan chan struct{}) error {
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}

	ctx, t.cancel = context.WithCancel(ctx)

	go func() {
		if err := t.client.PodPortForward(ctx, t.namespace, t.name, t.address, t.ports, readyChan); err != nil {
			slog.ErrorContext(ctx, "failed to forward", "address", t.address, "ports", t.ports, "error", err)
		}
	}()

	return nil
}

func (t *tunnel) Stop() error {
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}

	return nil
}
