package ssh

import (
	"context"
	"fmt"
	"io"
	"net"

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

	client, err := ssh.Dial("tcp", c.addr, config)

	if err != nil {
		return err
	}

	defer client.Close()

	session, err := client.NewSession()

	if err != nil {
		return err
	}

	session.Stdin = c.stdin
	session.Stdout = c.stdout
	session.Stderr = c.stderr

	defer session.Close()

	for _, p := range c.localPortForwards {
		listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", p.LocalAddr, p.LocalPort))

		if err != nil {
			return err
		}

		defer listener.Close()

		go tunnelConnections(listener, client, fmt.Sprintf("%s:%d", p.RemoteAddr, p.RemotePort))
	}

	for _, p := range c.remotePortForwards {
		listener, err := client.Listen("tcp", fmt.Sprintf("%s:%d", p.RemoteAddr, p.RemotePort))

		if err != nil {
			return err
		}

		defer listener.Close()

		go tunnelConnections(listener, &net.Dialer{}, fmt.Sprintf("%s:%d", p.LocalAddr, p.LocalPort))
	}

	if c.command != "" {
		return session.Run(c.command)
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

		defer conn.Close()

		go tunnelConnection(conn, d, addr)
	}
}

func tunnelConnection(c net.Conn, d dialer, addr string) error {
	conn, err := d.Dial("tcp", addr)

	if err != nil {
		return err
	}

	defer conn.Close()

	return tunnel(conn, c)
}

func tunnel(local, remote net.Conn) error {
	defer local.Close()
	defer remote.Close()

	result := make(chan error, 2)

	go func() {
		_, err := io.Copy(local, remote)
		result <- err
	}()

	go func() {
		_, err := io.Copy(remote, local)
		result <- err
	}()

	return <-result
}
