package run

import (
	"context"
	"fmt"
	"log"

	"github.com/adrianliechti/loop/pkg/sftp"
)

func startServer(ctx context.Context, port int, mounts []sftp.Mount) error {
	s, err := sftp.NewServer(fmt.Sprintf("127.0.0.1:%d", port), "/", mounts...)

	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		s.Close()
	}()

	go func() {
		if err := s.ListenAndServe(); err != nil {
			log.Println("could not start server", "error", err)
		}
	}()

	return nil
}
