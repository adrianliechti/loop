package sftp

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
)

type Server struct {
	server *ssh.Server
}

func NewServer(addr, path string) *Server {
	s := &ssh.Server{
		Addr: addr,

		Handler: func(s ssh.Session) {
			io.WriteString(s, "SFTP server ready. Use SFTP for file transfer.\n")
		},

		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": func(s ssh.Session) {

				srv := NewRequestServer(s, path)

				if err := srv.Serve(); err != nil {
					srv.Close()
				}
			},
		},
	}

	return &Server{
		server: s,
	}
}

func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		if errors.Is(err, ssh.ErrServerClosed) {
			return nil
		}

		return err
	}

	if err := s.server.Close(); err != nil {
		if errors.Is(err, ssh.ErrServerClosed) {
			return nil
		}

		return err
	}

	return nil
}

func (s *Server) ListenAndServe() error {
	if err := s.server.ListenAndServe(); err != nil {
		if errors.Is(err, ssh.ErrServerClosed) {
			return nil
		}

		return err
	}

	return nil
}

type RequestServer struct {
	root string

	server *sftp.RequestServer
}

func NewRequestServer(session io.ReadWriteCloser, root string) *RequestServer {
	handler := &handler{root}

	s := sftp.NewRequestServer(session, sftp.Handlers{
		FileGet:  handler,
		FilePut:  handler,
		FileCmd:  handler,
		FileList: handler,
	})

	return &RequestServer{
		root: root,

		server: s,
	}
}

func (s *RequestServer) Serve() error {
	return s.server.Serve()
}

func (s *RequestServer) Close() error {
	return s.server.Close()
}
