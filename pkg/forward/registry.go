package forward

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/adrianliechti/loop/pkg/system"
)

// refreshInterval is how often the desired tunnel set is recomputed. Informers
// keep the underlying caches current; this only bounds how long a change takes
// to become a tunnel.
const refreshInterval = 10 * time.Second

// Registry owns the running tunnels and the /etc/hosts section describing them.
type Registry struct {
	hosts *system.HostsSection

	logger *slog.Logger

	addFunc    func(address string, hosts []string, ports []int)
	deleteFunc func(address string, hosts []string, ports []int)

	tunnels []*Tunnel
}

func NewRegistry(section string, logger *slog.Logger, addFunc, deleteFunc func(address string, hosts []string, ports []int)) (*Registry, error) {
	hosts, err := system.NewHostsSection(section)

	if err != nil {
		return nil, err
	}

	return &Registry{
		hosts: hosts,

		logger: logger,

		addFunc:    addFunc,
		deleteFunc: deleteFunc,
	}, nil
}

// Reset drops the hosts section, discarding entries a previous run may have
// left behind. Callers do this before starting so a crashed session's stale
// entries are cleaned up even if this one never gets as far as Run.
func (r *Registry) Reset() {
	r.hosts.Clear()
	r.hosts.Flush()
}

// Run reconciles against list until ctx is cancelled, then tears down every
// tunnel and clears the hosts section.
func (r *Registry) Run(ctx context.Context, list func(ctx context.Context) ([]*Tunnel, error)) error {
	defer func() {
		r.Reset()

		for _, t := range r.tunnels {
			t.Stop()
		}
	}()

	if err := r.refresh(ctx, list); err != nil {
		return err
	}

	for {
		select {
		case <-time.After(refreshInterval):
		case <-ctx.Done():
			return nil
		}

		if err := r.refresh(ctx, list); err != nil && r.logger != nil {
			r.logger.ErrorContext(ctx, "refresh failed", "error", err)
		}
	}
}

func (r *Registry) refresh(ctx context.Context, list func(ctx context.Context) ([]*Tunnel, error)) error {
	desired, err := list(ctx)

	if err != nil {
		return err
	}

	var result error

	desiredByAddr := make(map[string]*Tunnel, len(desired))

	for _, t := range desired {
		desiredByAddr[t.address] = t
	}

	previousByAddr := make(map[string]*Tunnel, len(r.tunnels))

	for _, t := range r.tunnels {
		previousByAddr[t.address] = t
	}

	// Stop tunnels whose address is gone, OR whose descriptor changed
	// (different hosts/ports/target pod). Anything still equivalent is reused.
	for addr, t := range previousByAddr {
		if desiredT, kept := desiredByAddr[addr]; kept && t.equivalent(desiredT) {
			continue
		}

		r.hosts.Remove(t.address)

		if err := t.Stop(); err != nil {
			result = errors.Join(result, err)
			continue
		}

		if r.deleteFunc != nil {
			r.deleteFunc(t.address, t.hosts, slices.Collect(maps.Keys(t.ports)))
		}
	}

	// Build the new active tunnel set. Reuse the prior tunnel object (keeping
	// its running goroutine + cancel func) only when the descriptor is
	// equivalent; otherwise start the new one.
	next := make([]*Tunnel, 0, len(desired))

	for _, t := range desired {
		if prev, ok := previousByAddr[t.address]; ok && prev.equivalent(t) {
			next = append(next, prev)
			continue
		}

		if err := t.Start(ctx); err != nil {
			result = errors.Join(result, err)
			continue
		}

		r.hosts.Add(t.address, t.hosts...)

		if r.addFunc != nil {
			r.addFunc(t.address, t.hosts, slices.Collect(maps.Keys(t.ports)))
		}

		next = append(next, t)
	}

	r.tunnels = next

	// Do not flush if context is cancelled - let defer cleanup handle it
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if err := r.hosts.Flush(); err != nil {
		return errors.Join(result, err)
	}

	return result
}
