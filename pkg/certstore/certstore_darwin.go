//go:build darwin

package certstore

import (
	"context"
	"os"
	"os/exec"
)

func AddRootCA(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", "/Library/Keychains/System.keychain", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func RemoveRootCA(ctx context.Context, name string) error {
	fingerprint, err := certFingerprint(name)

	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "security", "delete-certificate", "-t", "-Z", fingerprint, "/Library/Keychains/System.keychain")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
