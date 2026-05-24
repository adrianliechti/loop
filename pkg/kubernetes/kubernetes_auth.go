package kubernetes

import (
	"net/http"
	"net/url"
	"strings"

	"k8s.io/client-go/transport"
)

// ensureScheme returns host with an https:// scheme prepended if no scheme is
// present. rest.Config.Host can be a bare "host:port" (e.g. "localhost:6443"),
// which url.Parse would otherwise mis-parse as scheme="localhost".
func ensureScheme(host string) string {
	if strings.Contains(host, "://") {
		return host
	}

	return "https://" + host
}

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

	u, err := url.Parse(ensureScheme(rc.Host))

	if err != nil {
		return nil, err
	}

	host := u.Hostname()
	port := u.Port()

	if host == "" {
		host = "localhost"
	}

	if port == "" {
		port = "443"
	}

	if rc.APIPath != "" {
		u = u.JoinPath(rc.APIPath)
	}

	result := &Credentials{
		Host: host,
		Port: port,

		URL: u,

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

	req, err := http.NewRequest("GET", u.String(), nil)

	if err != nil {
		return nil, err
	}

	resp, err := rt.RoundTrip(req)

	if err != nil {
		return nil, err
	}

	header := resp.Request.Header.Get("Authorization")

	if strings.HasPrefix(header, "Bearer ") {
		result.Token = strings.TrimPrefix(header, "Bearer ")
	}

	return result, nil
}

type authWrapper struct{}

func (rt *authWrapper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		Request: req,
	}, nil
}
