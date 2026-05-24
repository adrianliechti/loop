//go:build darwin || linux

package system

import "os"

func IsElevated() (bool, error) {
	return os.Geteuid() == 0, nil
}
