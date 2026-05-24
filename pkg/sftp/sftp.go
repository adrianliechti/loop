package sftp

import (
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net"
	"os"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Server struct {
	addr string

	// root is opened once in NewServer and never reassigned, so handlers can
	// read it without synchronization. *os.Root itself is safe to Close
	// concurrently with in-flight operations: pending ops complete or return
	// fs.ErrClosed, and the fd is only released afterwards (no reuse race).
	root *os.Root

	config *ssh.ServerConfig
}

func NewServer(addr, root string) (*Server, error) {
	r, err := os.OpenRoot(root)

	if err != nil {
		return nil, err
	}

	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			return nil, nil
		},
	}

	signer, err := generateSigner()

	if err != nil {
		r.Close()
		return nil, err
	}

	config.AddHostKey(signer)

	return &Server{
		addr:   addr,
		root:   r,
		config: config,
	}, nil
}

// Close releases the root directory. In-flight handlers that touch the root
// after Close will see fs.ErrClosed on their next operation and unwind on
// their own; Close does not wait for them to do so, which avoids parking
// shutdown on a long-lived sshfs (or similar) client. The listener used by
// Serve is owned by the caller; close it separately to stop the accept loop.
func (s *Server) Close() error {
	if s.root == nil {
		return nil
	}

	return s.root.Close()
}

// Serve accepts connections on the supplied listener until it is closed.
// The caller owns l and is responsible for closing it.
func (s *Server) Serve(l net.Listener) error {
	for {
		c, err := l.Accept()

		if err != nil {
			return err
		}

		go s.HandleConn(c)
	}
}

// ListenAndServe binds the address passed to NewServer and serves on it.
// Callers that need synchronous bind-error handling should call net.Listen
// themselves and pass the listener to Serve.
func (s *Server) ListenAndServe() error {
	l, err := net.Listen("tcp", s.addr)

	if err != nil {
		return err
	}

	defer l.Close()

	return s.Serve(l)
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
				if req.Type == "subsystem" && subsystemName(req.Payload) == "sftp" {
					req.Reply(true, nil)
					continue
				}

				req.Reply(false, nil)
			}
		}(requests)

		server := NewRequestServer(channel, s.root)

		if err := server.Serve(); err != nil {
			return err
		}

		server.Close()
	}

	return nil
}

// subsystemName extracts the subsystem name from an SSH subsystem-request
// payload (4-byte length prefix followed by the name), returning "" on a
// short payload instead of panicking on the slice.
func subsystemName(payload []byte) string {
	if len(payload) < 4 {
		return ""
	}

	return string(payload[4:])
}

func generateSigner() (ssh.Signer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)

	if err != nil {
		return nil, err
	}

	return ssh.NewSignerFromKey(key)
}

type RequestServer struct {
	server *sftp.RequestServer
}

func NewRequestServer(session io.ReadWriteCloser, root *os.Root) *RequestServer {
	h := &handler{root: root}

	s := sftp.NewRequestServer(session, sftp.Handlers{
		FileGet:  h,
		FilePut:  h,
		FileCmd:  h,
		FileList: h,
	})

	return &RequestServer{
		server: s,
	}
}

func (s *RequestServer) Serve() error {
	return s.server.Serve()
}

func (s *RequestServer) Close() error {
	return s.server.Close()
}
