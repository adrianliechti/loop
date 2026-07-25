//go:build linux

package certstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func AddRootCA(ctx context.Context, name string) error {
	path, err := certPath(name)

	if err != nil {
		return err
	}

	data, err := os.ReadFile(name)

	if err != nil {
		return err
	}

	tee := exec.CommandContext(ctx, "sudo", "tee", path)
	tee.Stdin = bytes.NewReader(data)
	tee.Stdout = os.Stdout
	tee.Stderr = os.Stderr

	if err := tee.Run(); err != nil {
		return err
	}

	tool, err := certTool()

	if err != nil {
		return err
	}

	update := exec.CommandContext(ctx, "sudo", tool...)
	update.Stdin = os.Stdin
	update.Stdout = os.Stdout
	update.Stderr = os.Stderr

	return update.Run()
}

func RemoveRootCA(ctx context.Context, name string) error {
	path, err := certPath(name)

	if err != nil {
		return err
	}

	rm := exec.CommandContext(ctx, "sudo", "rm", "-f", path)
	rm.Stdin = os.Stdin
	rm.Stdout = os.Stdout
	rm.Stderr = os.Stderr

	if err := rm.Run(); err != nil {
		return err
	}

	tool, err := certTool()

	if err != nil {
		return err
	}

	update := exec.CommandContext(ctx, "sudo", tool...)
	update.Stdin = os.Stdin
	update.Stdout = os.Stdout
	update.Stderr = os.Stderr

	return update.Run()
}

// trustStores describes where each distribution family keeps its local trust
// anchors, in probe order. The first one whose directory exists wins.
var trustStores = []struct {
	dir       string
	extension string
	tool      []string
}{
	{"/etc/pki/ca-trust/source/anchors/", "pem", []string{"update-ca-trust", "extract"}},
	{"/usr/local/share/ca-certificates/", "crt", []string{"update-ca-certificates"}},
	{"/etc/ca-certificates/trust-source/anchors/", "crt", []string{"trust", "extract-compat"}},
	{"/usr/share/pki/trust/anchors/", "pem", []string{"update-ca-certificates"}},
}

func trustStore() (dir, extension string, tool []string, err error) {
	for _, s := range trustStores {
		if _, err := os.Stat(s.dir); err == nil {
			return s.dir, s.extension, s.tool, nil
		}
	}

	return "", "", nil, errors.New("could not determine certificate path")
}

func certPath(name string) (string, error) {
	fingerprint, err := certFingerprint(name)

	if err != nil {
		return "", err
	}

	dir, extension, _, err := trustStore()

	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%sca-%s.%s", dir, fingerprint, extension), nil
}

func certTool() ([]string, error) {
	_, _, tool, err := trustStore()

	return tool, err
}
