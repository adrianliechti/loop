package system

import (
	"net"
	"strconv"
)

func FreePort(preference string) (string, error) {
	if port, err := freePort(preference); err == nil {
		return port, err
	}

	return freePort("")
}

func freePort(port string) (string, error) {
	if port == "" {
		port = "0"
	}

	addr, err := net.ResolveTCPAddr("tcp", "localhost:"+port)

	if err != nil {
		return "", err
	}

	ln, err := net.ListenTCP("tcp", addr)

	if err != nil {
		return "", err
	}

	defer ln.Close()

	result := ln.Addr().(*net.TCPAddr).Port
	return strconv.Itoa(result), nil
}
