package docker

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
)

func Login(ctx context.Context, address, username, password string) error {
	tool, _, err := Tool(ctx)

	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, tool, "login", "--username", username, "--password-stdin", address)
	cmd.Stdin = bytes.NewReader([]byte(password))

	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.New(strings.TrimSpace(string(output)))
	}

	return nil
}
