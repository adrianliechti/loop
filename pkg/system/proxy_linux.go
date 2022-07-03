//go:build linux

package system

func SetSocksProxy(host, port string) error {
	return nil
}

func ResetSocksProxy() error {
	return nil
}
