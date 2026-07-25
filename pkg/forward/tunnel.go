// Package forward maps cluster addresses onto local loopback aliases and keeps
// a set of port-forwards in sync with a desired state. It is shared by the
// catapult (service/pod addresses) and gateway (ingress/route hostnames)
// implementations, which differ only in how they derive that desired state.
package forward

import (
	"context"
	"crypto/md5"
	"io"
	"log/slog"
	"maps"
	"net"
	"slices"

	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/system"
)

// Tunnel forwards a set of ports on a dedicated loopback address to a pod.
type Tunnel struct {
	client kubernetes.Client

	namespace string
	name      string

	address string
	ports   map[int]int
	hosts   []string

	cancel context.CancelFunc
}

func NewTunnel(client kubernetes.Client, namespace, name, address string, ports map[int]int, hosts []string) *Tunnel {
	return &Tunnel{
		client: client,

		namespace: namespace,
		name:      name,

		address: address,
		ports:   ports,
		hosts:   hosts,
	}
}

func (t *Tunnel) Address() string {
	return t.address
}

func (t *Tunnel) Hosts() []string {
	return t.hosts
}

// AddHost registers an additional hostname resolving to this tunnel's address.
func (t *Tunnel) AddHost(host string) {
	t.hosts = append(t.hosts, host)
}

func (t *Tunnel) Start(ctx context.Context) error {
	t.stop()

	ctx, t.cancel = context.WithCancel(ctx)

	if err := system.AliasIP(ctx, t.address); err != nil {
		return err
	}

	go func() {
		if err := t.client.PodPortForward(ctx, t.namespace, t.name, t.address, t.ports, nil); err != nil {
			slog.ErrorContext(ctx, "failed to forward", "address", t.address, "ports", t.ports, "error", err)
		}
	}()

	return nil
}

func (t *Tunnel) Stop() error {
	t.stop()

	return system.UnaliasIP(context.Background(), t.address)
}

func (t *Tunnel) stop() {
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
}

// equivalent reports whether two tunnels target the same workload with the
// same hosts and port mappings — i.e. whether the running goroutine is still
// correct for the new desired state.
func (t *Tunnel) equivalent(o *Tunnel) bool {
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

// MapAddress derives a stable loopback address for a cluster address. The
// second octet separates callers so that, for example, catapult and gateway
// tunnels can never collide on the same alias.
func MapAddress(group byte, address string) string {
	h := md5.New()
	io.WriteString(h, address)

	addr := h.Sum(nil)[:4]
	addr[0] = 127
	addr[1] = group

	return net.IP(addr).String()
}
