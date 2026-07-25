package kubernetes

import (
	"context"
	"io"
	"os"
	"path"
)

func (c *client) ReadFileInPod(ctx context.Context, namespace, name, container, path string, data io.Writer) error {
	cat := []string{
		"cat",
		path,
	}

	return c.PodExec(ctx, namespace, name, container, cat, false, nil, data, os.Stderr)
}

func (c *client) CreateFileInPod(ctx context.Context, namespace, name, container, containerPath string, data io.Reader) error {
	mkdir := []string{
		"mkdir",
		"-p",
		path.Dir(containerPath),
	}

	if err := c.PodExec(ctx, namespace, name, container, mkdir, false, nil, os.Stdout, os.Stderr); err != nil {
		return err
	}

	cp := []string{
		"cp",
		"/dev/stdin",
		containerPath,
	}

	return c.PodExec(ctx, namespace, name, container, cp, false, data, os.Stdout, os.Stderr)
}
