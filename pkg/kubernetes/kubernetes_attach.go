package kubernetes

import (
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func (c *client) PodAttach(ctx context.Context, namespace, name, container string, tty bool, stdin io.Reader, stdout, stderr io.Writer) error {
	return c.podStream(ctx, namespace, name, container, "attach", tty, stdin, stdout, stderr, func(container string) runtime.Object {
		return &corev1.PodAttachOptions{
			Container: container,
			TTY:       tty,

			Stdin:  stdin != nil,
			Stdout: stdout != nil,
			Stderr: stderr != nil,
		}
	})
}
