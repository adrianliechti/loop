package docker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/ssh"
	"github.com/adrianliechti/loop/pkg/system"
)

type ConnectOptions struct {
	Namespace string

	Port int

	SyncMode SyncMode

	Ports   []Port
	Volumes []Volume
}

func Connect(ctx context.Context, client kubernetes.Client, name string, options *ConnectOptions) error {
	if options == nil {
		options = new(ConnectOptions)
	}

	if options.Namespace == "" {
		options.Namespace = client.Namespace()
	}

	if options.Port == 0 {
		port, err := system.FreePort(2375)

		if err != nil {
			return err
		}

		options.Port = port
	}

	if options.SyncMode == SyncModeMount && len(options.Volumes) > 1 {
		return errors.New("mount mode currently only supports a single volume")
	}

	cli.Infof("★ Connecting to Docker instance '%s'", name)

	podName := resourceName(name) + "-0"

	if _, err := client.WaitForPod(ctx, options.Namespace, podName); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Port forward to SSH on the tunnel container
	sshPort, err := system.FreePort(0)

	if err != nil {
		return err
	}

	sshReady := make(chan struct{})

	go func() {
		client.PodPortForward(ctx, options.Namespace, podName, "", map[int]int{sshPort: 22}, sshReady)
		cancel()
	}()

	<-sshReady

	sshAddr := fmt.Sprintf("127.0.0.1:%d", sshPort)

	// Tunnel Docker port through SSH
	sshOptions := []ssh.Option{
		ssh.WithLocalPortForward(ssh.PortForward{LocalPort: options.Port, RemotePort: 2375}),
	}

	// Tunnel additional user ports through SSH
	for _, p := range options.Ports {
		sshOptions = append(sshOptions, ssh.WithLocalPortForward(ssh.PortForward{LocalPort: p.Source, RemotePort: p.Target}))
	}

	// Mount volumes through SFTP
	if len(options.Volumes) > 0 && options.SyncMode == SyncModeMount {
		volume := options.Volumes[0]

		sftpPort, err := system.FreePort(0)

		if err != nil {
			return err
		}

		if err := startServer(ctx, sftpPort, volume.Source); err != nil {
			return err
		}

		targetPath := path.Join("/data", volume.Target)

		sshOptions = append(sshOptions,
			ssh.WithRemotePortForward(ssh.PortForward{LocalPort: sftpPort, RemotePort: 2222}),
			ssh.WithCommand("sshfs -o allow_other -p 2222 root@localhost:/ "+targetPath+" && /bin/sleep infinity"),
			ssh.WithStderr(os.Stderr),
			ssh.WithStdout(os.Stdout),
		)
	}

	go func() {
		sshClient := ssh.New(sshAddr, sshOptions...)

		if err := sshClient.Run(ctx); err != nil {
			cli.Error(err)
		}

		cancel()
	}()

	// Setup Docker context
	docker := "docker"
	loopContext := "loop-" + name
	currentContext := "default"

	if val, err := exec.Command(docker, "context", "show").Output(); err == nil {
		currentContext = strings.TrimRight(string(val), "\n")
	}

	defer func() {
		cli.Info("★ Resetting Docker context to '" + currentContext + "'")
		exec.Command(docker, "context", "use", currentContext).Run()
		exec.Command(docker, "context", "rm", loopContext).Run()
	}()

	cli.Info("★ Setting Docker context to '" + loopContext + "'")
	exec.Command(docker, "context", "rm", loopContext).Run()
	exec.Command(docker, "context", "create", loopContext, "--docker", fmt.Sprintf("host=tcp://127.0.0.1:%d", options.Port)).Run()
	exec.Command(docker, "context", "use", loopContext).Run()

	cli.Info("★ Press Ctrl+C to disconnect")

	// Wait for context cancellation
	<-ctx.Done()

	return nil
}
