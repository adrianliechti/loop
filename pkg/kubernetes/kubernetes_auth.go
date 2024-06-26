package kubernetes

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"k8s.io/client-go/transport"
)

type Credentials struct {
	Host string
	Port string

	CAData  []byte
	APIPath string

	Token string
}

func (c *client) Credentials() (*Credentials, error) {
	rc := c.Config()

	host := "localhost"
	port := "443"

	if h, p, err := net.SplitHostPort(host); err == nil {
		host = h
		port = p
	}

	if val, err := url.Parse(rc.Host); err == nil {
		port = val.Port()

		if port == "" {
			port = "443"
		}

		host = val.Hostname()

		if host == "" {
			host = "localhost"
		}
	}

	result := &Credentials{
		Host: host,
		Port: port,

		CAData:  rc.CAData,
		APIPath: rc.APIPath,
	}

	tc, err := rc.TransportConfig()

	if err != nil {
		return nil, err
	}

	wrapper := &authWrapper{}

	rt, err := transport.HTTPWrappersForConfig(tc, wrapper)

	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequest("GET", "http://localhost:8080/api", nil)
	resp, err := rt.RoundTrip(req)

	if err != nil {
		return nil, err
	}

	header := resp.Request.Header.Get("Authorization")

	if strings.HasPrefix(header, "Bearer ") {
		result.Token = header[7:]
	}

	return result, nil
}

type authWrapper struct {
}

func (rt *authWrapper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		Request: req,
	}, nil
}
