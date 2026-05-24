package gateway

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"slices"

	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/system"
)

type tunnel struct {
	client kubernetes.Client

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

		ports: ports,
		hosts: hosts,
	}
}

func (t *tunnel) Start(ctx context.Context, readyChan chan struct{}) error {
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}

	ctx, t.cancel = context.WithCancel(ctx)

	if err := system.AliasIP(ctx, t.address); err != nil {
		return err
	}

	go func() {
		if err := t.client.PodPortForward(ctx, t.namespace, t.name, t.address, t.ports, readyChan); err != nil {
			slog.ErrorContext(ctx, "failed to forward", "address", t.address, "ports", t.ports, "error", err)
		}
	}()

	return nil
}

// equivalent reports whether two tunnels target the same workload with the
// same hosts and port mappings — i.e. whether the running goroutine is still
// correct for the new desired state.
func (t *tunnel) equivalent(o *tunnel) bool {
	if t.namespace != o.namespace || t.name != o.name {
		return false
	}

	if !maps.Equal(t.ports, o.ports) {
		return false
	}

	a := slices.Clone(t.hosts)
	b := slices.Clone(o.hosts)
	slices.Sort(a)
	slices.Sort(b)

	return slices.Equal(a, b)
}

func (t *tunnel) Stop() error {
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}

	var result error

	if err := system.UnaliasIP(context.Background(), t.address); err != nil {
		result = errors.Join(result, err)
	}

	return result
}
