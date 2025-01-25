package os

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"

	"github.com/adrianliechti/loop/pkg/fs"
)

var _ fs.Watcher = &Watcher{}

type Watcher struct {
	watcher *fsnotify.Watcher
}

func NewWatcher() (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()

	if err != nil {
		return nil, err
	}

	return &Watcher{watcher}, nil
}

func (w *Watcher) Watch(ctx context.Context, path ...string) (<-chan fs.Event, error) {
	events := make(chan fs.Event)

	for _, p := range path {
		err := filepath.Walk(p, func(name string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if strings.Contains(name, ".git") {
				return nil
			}

			if info.IsDir() {
				w.watcher.Add(name)
			}

			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	go func() {
		for {
			select {
			case event := <-w.watcher.Events:
				info, err := os.Stat(event.Name)

				if os.IsNotExist(err) {
					w.watcher.Remove(event.Name)
				}

				if info != nil && info.IsDir() {
					w.watcher.Add(event.Name)
				}

				switch event.Op {
				case fsnotify.Create:
					events <- fs.Event{
						Action: fs.Create,
						Path:   event.Name,
					}

				case fsnotify.Write:
					events <- fs.Event{
						Action: fs.Modify,
						Path:   event.Name,
					}

				case fsnotify.Remove, fsnotify.Rename:
					events <- fs.Event{
						Action: fs.Remove,
						Path:   event.Name,
					}

				case fsnotify.Chmod:
					// ignore
				}

			case <-ctx.Done():
				w.watcher.Close()
				return
			}
		}
	}()

	return events, nil
}
