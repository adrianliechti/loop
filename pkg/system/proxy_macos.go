//go:build darwin

package system

import (
	"bufio"
	"bytes"
	"os/exec"
	"strings"

	"github.com/hashicorp/go-multierror"
)

func SetSocksProxy(host, port string) error {
	services, err := listServices()

	if err != nil {
		return err
	}

	var result error

	for _, service := range services {
		if err := setSocksProxy(service, host, port); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result
}

func ResetSocksProxy() error {
	return SetSocksProxy("", "")
}

func setSocksProxy(service, host, port string) error {
	state := "on"

	if port == "" {
		port = "1080"
	}

	if host == "" {
		port = ""
		state = "off"
	}

	if err := exec.Command("networksetup", "-setsocksfirewallproxy", service, host, port).Run(); err != nil {
		return err
	}

	if err := exec.Command("networksetup", "-setsocksfirewallproxystate", service, state).Run(); err != nil {
		return err
	}

	return nil
}

func listServices() ([]string, error) {
	/*
		USB 10/100/1000 LAN
		Wi-Fi
		iPhone USB
		Thunderbolt Bridge
		*Tailscale Tunnel
	*/

	var services []string

	cmd := exec.Command("networksetup", "-listallnetworkservices")
	output, err := cmd.Output()

	if err != nil {
		return services, err
	}

	scanner := bufio.NewScanner(bytes.NewBuffer(output))

	for scanner.Scan() {
		service := scanner.Text()

		if strings.Contains(service, "network service is disabled") {
			continue
		}

		service = strings.TrimLeft(service, "*")
		services = append(services, service)
	}

	return services, nil
}
