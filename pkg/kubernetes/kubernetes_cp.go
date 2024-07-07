package kubernetes

import (
	"context"
	"io"
	"os"
	"path"
)

func (c *client) ReadFileInPod(ctx context.Context, namespace, name, container, path string, data io.Writer) error {
	cp := []string{
		"cat",
		path,
	}

	if err := c.PodExec(ctx, namespace, name, container, cp, false, nil, data, os.Stderr); err != nil {
		return err
	}

	return nil
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

	if err := c.PodExec(ctx, namespace, name, container, cp, false, data, os.Stdout, os.Stderr); err != nil {
		return err
	}

	return nil
}
