package kubernetes

import (
	"context"
	"io"

	corev1 "k8s.io/api/core/v1"
)

func (c *client) PodLogs(ctx context.Context, namespace, name, container string, out io.Writer, follow bool) error {
	req := c.CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{
		Follow:    follow,
		Container: container,
	})

	reader, err := req.Stream(ctx)

	if err != nil {
		return err
	}

	defer reader.Close()

	if _, err := io.Copy(out, reader); err != nil {
		return err
	}

	return nil
}
