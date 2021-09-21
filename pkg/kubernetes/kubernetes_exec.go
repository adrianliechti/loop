package kubernetes

import (
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

func (c *client) PodExec(ctx context.Context, namespace, name, container string, command []string, tty bool, stdin io.Reader, stdout, stderr io.Writer) error {
	req := c.CoreV1().RESTClient().Post().Resource("pods").Name(name).Namespace(namespace).SubResource("exec")

	req.VersionedParams(
		&corev1.PodExecOptions{
			Container: container,
			TTY:       tty,

			Stdin:  stdin != nil,
			Stdout: stdout != nil,
			Stderr: stderr != nil,

			Command: command,
		},
		scheme.ParameterCodec,
	)

	exec, err := remotecommand.NewSPDYExecutor(c.Config(), "POST", req.URL())

	if err != nil {
		return err
	}

	streamOptions := remotecommand.StreamOptions{
		Tty: tty,

		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}

	return exec.Stream(streamOptions)
}
