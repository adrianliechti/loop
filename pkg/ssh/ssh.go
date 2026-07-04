package ssh

import (
	"context"
	"io"
	"net"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
)

type Client struct {
	addr string

	username string

	command string

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	localPortForwards  []PortForward
	remotePortForwards []PortForward

	ready chan<- struct{}
}

func New(addr string, options ...Option) *Client {
	c := &Client{
		addr: addr,

		username: "root",
	}

	for _, option := range options {
		option(c)
	}

	return c
}

type PortForward struct {
	LocalAddr string
	LocalPort int

	RemoteAddr string
	RemotePort int

	// BoundRemotePort, if non-nil, receives the port the remote side actually
	// bound before the ready channel is closed. Combined with RemotePort 0 this
	// lets sshd pick a free port instead of racing other sessions for a fixed
	// one. The channel must be buffered; only remote forwards report a port.
	BoundRemotePort chan<- int
}

type Option func(*Client)

func WithUsername(username string) Option {
	return func(c *Client) {
		c.username = username
	}
}

func WithStdin(r io.Reader) Option {
	return func(c *Client) {
		c.stdin = r
	}
}

func WithStdout(w io.Writer) Option {
	return func(c *Client) {
		c.stdout = w
	}
}

func WithStderr(w io.Writer) Option {
	return func(c *Client) {
		c.stderr = w
	}
}

func WithCommand(command string) Option {
	return func(c *Client) {
		c.command = command
	}
}

func WithLocalPortForward(p PortForward) Option {
	return func(c *Client) {
		if p.LocalAddr == "" {
			p.LocalAddr = "127.0.0.1"
		}

		if p.RemoteAddr == "" {
			p.RemoteAddr = "127.0.0.1"
		}

		c.localPortForwards = append(c.localPortForwards, p)
	}
}

// WithReady configures a channel that Run closes after the SSH session has
// been established and every local/remote port forward is actually listening.
// Callers can use this to delay signalling "ready" downstream (e.g. opening
// a browser) until the tunnels can actually accept connections.
func WithReady(ch chan<- struct{}) Option {
	return func(c *Client) {
		c.ready = ch
	}
}

func WithRemotePortForward(p PortForward) Option {
	return func(c *Client) {
		if p.LocalAddr == "" {
			p.LocalAddr = "127.0.0.1"
		}

		if p.RemoteAddr == "" {
			p.RemoteAddr = "127.0.0.1"
		}

		c.remotePortForwards = append(c.remotePortForwards, p)
	}
}

func (c *Client) Run(ctx context.Context) error {
	config := &ssh.ClientConfig{
		User: c.username,

		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// Everything below must unblock when ctx is cancelled: a half-open tunnel
	// (local listener up, backend gone) otherwise wedges the TCP dial, the SSH
	// handshake, or session setup forever — hanging shutdown paths that use a
	// bounded context. Closing the underlying conn/client aborts them all.
	dialer := &net.Dialer{Timeout: 15 * time.Second}

	conn, err := dialer.DialContext(ctx, "tcp", c.addr)

	if err != nil {
		return err
	}

	stopConn := context.AfterFunc(ctx, func() { conn.Close() })

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, c.addr, config)

	stopConn()

	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		return err
	}

	client := ssh.NewClient(sshConn, chans, reqs)

	defer client.Close()

	stop := context.AfterFunc(ctx, func() { client.Close() })
	defer stop()

	session, err := client.NewSession()

	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		return err
	}

	session.Stdin = c.stdin
	session.Stdout = c.stdout
	session.Stderr = c.stderr

	defer session.Close()

	for _, p := range c.localPortForwards {
		listener, err := net.Listen("tcp", net.JoinHostPort(p.LocalAddr, strconv.Itoa(p.LocalPort)))

		if err != nil {
			return err
		}

		defer listener.Close()

		go tunnelConnections(listener, client, net.JoinHostPort(p.RemoteAddr, strconv.Itoa(p.RemotePort)))
	}

	for _, p := range c.remotePortForwards {
		listener, err := client.Listen("tcp", net.JoinHostPort(p.RemoteAddr, strconv.Itoa(p.RemotePort)))

		if err != nil {
			return err
		}

		defer listener.Close()

		if p.BoundRemotePort != nil {
			p.BoundRemotePort <- listener.Addr().(*net.TCPAddr).Port
		}

		go tunnelConnections(listener, &net.Dialer{}, net.JoinHostPort(p.LocalAddr, strconv.Itoa(p.LocalPort)))
	}

	// All port forwards are bound at this point.
	if c.ready != nil {
		close(c.ready)
	}

	if c.command != "" {
		if err := session.Start(c.command); err != nil {
			return err
		}

		done := make(chan error, 1)

		go func() {
			done <- session.Wait()
		}()

		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			session.Close()
			client.Close()
			return ctx.Err()
		}
	}

	<-ctx.Done()
	return nil
}

type dialer interface {
	Dial(network, addr string) (net.Conn, error)
}

type listener interface {
	Accept() (net.Conn, error)
}

func tunnelConnections(l listener, d dialer, addr string) error {
	for {
		conn, err := l.Accept()

		if err != nil {
			return err
		}

		go tunnelConnection(conn, d, addr)
	}
}

func tunnelConnection(c net.Conn, d dialer, addr string) {
	defer c.Close()

	conn, err := d.Dial("tcp", addr)

	if err != nil {
		return
	}

	defer conn.Close()

	go io.Copy(conn, c)
	io.Copy(c, conn)
}
