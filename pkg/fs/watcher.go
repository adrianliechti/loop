package fs

import (
	"context"
)

type Watcher interface {
	Watch(ctx context.Context, path ...string) (<-chan Event, error)
}

type Action string

const (
	Create Action = "create"
	Modify Action = "modify"
	Remove Action = "remove"
)

type Event struct {
	Action Action

	Path string

	RenamedFrom string
}
