package sftp

import (
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Mount struct {
	Source string
	Target string
}

type Server struct {
	addr   string
	root   string
	mounts []Mount

	config *ssh.ServerConfig

	listener net.Listener
}

func NewServer(addr, root string, mounts ...Mount) (*Server, error) {
	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			return nil, nil
		},
	}

	signer, err := generateSigner()

	if err != nil {
		return nil, err
	}

	config.AddHostKey(signer)

	return &Server{
		addr:   addr,
		root:   root,
		mounts: mounts,

		config: config,
	}, nil
}

func (s *Server) Close() error {
	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}

	return nil
}

func (s *Server) ListenAndServe() error {
	l, err := net.Listen("tcp", s.addr)

	if err != nil {
		return err
	}

	for {
		c, err := l.Accept()

		if err != nil {
			return err
		}

		go s.HandleConn(c)
	}
}

func (s *Server) HandleConn(c net.Conn) error {
	_, chans, reqs, err := ssh.NewServerConn(c, s.config)

	if err != nil {
		return err
	}

	go ssh.DiscardRequests(reqs)

	for ch := range chans {
		if ch.ChannelType() != "session" {
			ch.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := ch.Accept()

		if err != nil {
			continue
		}

		go func(in <-chan *ssh.Request) {
			for req := range in {
				switch req.Type {
				case "subsystem":
					if string(req.Payload[4:]) == "sftp" {
						req.Reply(true, nil)
						continue
					}
				}

				req.Reply(false, nil)

			}
		}(requests)

		server := NewRequestServer(channel, s.root, s.mounts...)

		if err := server.Serve(); err != nil {
			return err
		}

		server.Close()
	}

	return nil
}

func generateSigner() (ssh.Signer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)

	if err != nil {
		return nil, err
	}

	return ssh.NewSignerFromKey(key)
}

type RequestServer struct {
	root   string
	mounts []Mount

	server *sftp.RequestServer
}

func NewRequestServer(session io.ReadWriteCloser, root string, mounts ...Mount) *RequestServer {
	handler := &handler{root: root, mounts: mounts}

	s := sftp.NewRequestServer(session, sftp.Handlers{
		FileGet:  handler,
		FilePut:  handler,
		FileCmd:  handler,
		FileList: handler,
	})

	return &RequestServer{
		root:   root,
		mounts: mounts,

		server: s,
	}
}

func (s *RequestServer) Serve() error {
	return s.server.Serve()
}

func (s *RequestServer) Close() error {
	return s.server.Close()
}
