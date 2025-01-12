package sftp

import (
	"bufio"
	"context"
	"encoding/json"
	"strings"

	"github.com/adrianliechti/loop/pkg/fs"

	"golang.org/x/crypto/ssh"
)

var _ fs.Watcher = &Watcher{}

type Watcher struct {
	client *ssh.Client
}

func NewWatcher(client *ssh.Client) (*Watcher, error) {
	return &Watcher{
		client: client,
	}, nil
}

func (w *Watcher) Watch(ctx context.Context, path ...string) (<-chan fs.Event, error) {
	events := make(chan fs.Event)

	session, err := w.client.NewSession()

	if err != nil {
		return nil, err
	}

	stdout, _ := session.StdoutPipe()

	go func() {
		cmd := "inotifywait -q -m -r --format '{\"type\":\"%e\", \"path\":\"%w%f\"}' -e create -e modify -e move -e delete -e attrib " + strings.Join(path, " ")
		err := session.Run(cmd)
		_ = err
		close(events)
	}()

	go func() {
		<-ctx.Done()
		session.Close()
	}()

	go func() {
		scanner := bufio.NewScanner(stdout)

		type eventType struct {
			Type string `json:"type"`
			Path string `json:"path"`
		}

		var moved string

		for scanner.Scan() {
			var event eventType

			if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
				continue
			}

			switch strings.Split(strings.ToLower(event.Type), ",")[0] {
			case "create":
				events <- fs.Event{
					Action: fs.Create,
					Path:   event.Path,
				}

			case "modify":
				events <- fs.Event{
					Action: fs.Modify,
					Path:   event.Path,
				}

			case "delete":
				events <- fs.Event{
					Action: fs.Remove,

					Path: event.Path,
				}

			case "moved_from":
				moved = event.Path

			case "moved_to":
				events <- fs.Event{
					Action: fs.Create,

					Path:        event.Path,
					RenamedFrom: moved,
				}

			case "attrib":
				// ignore
			}
		}
	}()

	return events, nil
}
