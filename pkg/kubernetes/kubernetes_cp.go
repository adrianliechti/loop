package kubernetes

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

func (c *client) CreateFileInPod(ctx context.Context, namespace, name, container, path string, data io.Reader) error {
	mkdir := []string{
		"mkdir",
		"-p",
		filepath.Dir(path),
	}

	if err := c.PodExec(ctx, namespace, name, container, mkdir, false, nil, os.Stdout, os.Stderr); err != nil {
		return err
	}

	cp := []string{
		"cp",
		"/dev/stdin",
		path,
	}

	if err := c.PodExec(ctx, namespace, name, container, cp, false, data, os.Stdout, os.Stderr); err != nil {
		return err
	}

	return nil
}
