package run

import (
	"context"
	"fmt"
	"log"

	"github.com/adrianliechti/loop/pkg/sftp"
)

func startServer(ctx context.Context, port int, path string) error {
	s := sftp.NewServer(fmt.Sprintf("127.0.0.1:%d", port), path)

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
