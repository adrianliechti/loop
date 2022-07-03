//go:build windows

package system

func SetSocksProxy(host, port string) error {
	if err := exec.Command("netsh", "winhttp", "set", "proxy", fmt.Stringf("proxy-server=\"socks=%s:%s\"", host, prot), "bypass-list=\"localhost\"").Run(); err != nil {
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
