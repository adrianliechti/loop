package kubernetes

import (
	"context"
	"errors"
	"net"
	"regexp"

	"github.com/Masterminds/semver/v3"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *client) Version(ctx context.Context) (*semver.Version, error) {
	version, err := c.Discovery().ServerVersion()

	if err != nil {
		return nil, err
	}

	return semver.NewVersion(version.GitVersion)
}

func (c *client) PodCIDR(ctx context.Context) (string, error) {
	nodes, err := c.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	if err != nil {
		return "", err
	}

	if len(nodes.Items) == 0 {
		return "", errors.New("no nodes found")
	}

	node := nodes.Items[0]
	cidr := node.Spec.PodCIDR

	addr, _, err := net.ParseCIDR(cidr)

	if err != nil {
		return "", err
	}

	if addr.To4() == nil {
		return "", errors.New("IPv6 not supported")
	}

	return cidr, nil
}

func (c *client) ServiceCIDR(ctx context.Context) (string, error) {
	_, err := c.CoreV1().Services("default").Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dummy",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "1.1.1.1",
			Ports: []corev1.ServicePort{
				{
					Port: 80,
				},
			},
		},
	}, metav1.CreateOptions{
		DryRun: []string{"All"},
	})

	if err == nil {
		return "", errors.New("unable to determine Service CIDR")
	}

	r, _ := regexp.Compile(`valid IPs is ([.\/0-9]+)`)
	matches := r.FindStringSubmatch(err.Error())

	if len(matches) != 2 {
		return "", errors.New("unable to determine Service CIDR")
	}

	cidr := matches[1]

	addr, _, err := net.ParseCIDR(cidr)

	if err != nil {
		return "", err
	}

	if addr.To4() == nil {
		return "", errors.New("IPv6 not supported")
	}

	return cidr, nil
}
