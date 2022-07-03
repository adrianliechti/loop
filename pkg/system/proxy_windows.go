//go:build windows

package system

import (
	"fmt"
	"os/exec"
)

func SetSocksProxy(host, port string) error {
	if err := exec.Command("netsh", "winhttp", "set", "proxy", fmt.Sprintf("proxy-server=\"socks=%s:%s\"", host, port), "bypass-list=\"localhost\"").Run(); err != nil {
		return err
	}

	return nil
}

func ResetSocksProxy() error {
	if err := exec.Command("netsh", "winhttp", "reset", "proxy").Run(); err != nil {
		return err
	}

	return nil
}
