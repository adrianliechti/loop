package system

import (
	"fmt"
	"net"
)

func FreePort(preference int) (int, error) {
	if port, err := freePort(preference); err == nil {
		return port, err
	}

	return freePort(0)
}

func freePort(port int) (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", port))

	if err != nil {
		return 0, err
	}

	ln, err := net.ListenTCP("tcp", addr)

	if err != nil {
		return 0, err
	}

	defer ln.Close()

	result := ln.Addr().(*net.TCPAddr).Port
	return result, nil
}
