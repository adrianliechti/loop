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

	URL *url.URL

	CAData   []byte
	KeyData  []byte
	CertData []byte

	Token string
}

func (c *client) Credentials() (*Credentials, error) {
	rc := c.Config()

	host := "localhost"
	port := "443"

	var url *url.URL

	if h, p, err := net.SplitHostPort(rc.Host); err == nil {
		host = h
		port = p

		url, _ = url.Parse("https://" + net.JoinHostPort(h, p))
		url = url.JoinPath(rc.APIPath)
	}

	if val, err := url.Parse(rc.Host); err == nil {
		if h := val.Hostname(); h != "" {
			host = h
		}

		if p := val.Port(); p != "" {
			port = p
		}

		url = val
	}

	result := &Credentials{
		Host: host,
		Port: port,

		URL: url,

		CAData:   rc.CAData,
		KeyData:  rc.KeyData,
		CertData: rc.CertData,
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

	req, _ := http.NewRequest("GET", url.String(), nil)
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
