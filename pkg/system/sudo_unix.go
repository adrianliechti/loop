//go:build darwin || linux

package system

import (
	"os/exec"
	"strings"
)

func IsElevated() (bool, error) {
	cmd := exec.Command("id", "-u")
	output, err := cmd.Output()

	if err != nil {
		return false, err
	}

	id := string(output)

	id = strings.TrimRight(id, "\n\r")
	id = strings.TrimSpace(id)

	if id == "0" {
		return true, nil
	}

	return false, nil
}
