package run

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"

	"github.com/adrianliechti/loop/pkg/sftp"
)

func startServer(ctx context.Context, port int, mounts []sftp.Mount) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	// Mounts-only server (empty root): only the volume targets exist.
	s, err := sftp.NewServer(addr, "", mounts...)

	if err != nil {
		return err
	}

	// Bind synchronously so bind failures (race on the port, EADDRINUSE) are
	// surfaced to the caller instead of swallowed by a background goroutine.
	l, err := net.Listen("tcp", addr)

	if err != nil {
		s.Close()
		return err
	}

	go func() {
		<-ctx.Done()
		l.Close()
		s.Close()
	}()

	go func() {
		if err := s.Serve(l); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Println("could not serve sftp", "error", err)
		}
	}()

	return nil
}
