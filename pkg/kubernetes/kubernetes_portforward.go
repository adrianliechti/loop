package kubernetes

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

func (c *client) ServicePortForward(ctx context.Context, namespace, name, address string, ports map[int]int, readyChan chan struct{}) error {
	service, err := c.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})

	if err != nil {
		return err
	}

	pod, err := c.ServicePod(ctx, namespace, name)

	if err != nil {
		return err
	}

	servicePorts := make(map[int]int)

	for _, port := range service.Spec.Ports {
		servicePort := int(port.Port)
		containerPort := 0

		if !(port.Protocol == corev1.ProtocolTCP || port.Protocol == corev1.ProtocolUDP) {
			continue
		}

		for _, c := range pod.Spec.Containers {
			for _, p := range c.Ports {
				if p.Name != "" && p.Name == port.TargetPort.String() {
					containerPort = int(p.ContainerPort)
				}
			}
		}

		if port.TargetPort.IntVal > 0 {
			containerPort = int(port.TargetPort.IntVal)
		}

		if servicePort > 0 && containerPort > 0 {
			servicePorts[servicePort] = containerPort
		}
	}

	mappings := make(map[int]int)

	for k, v := range ports {
		localPort := k
		targetPort := v

		if val, ok := servicePorts[targetPort]; ok {
			targetPort = val
		}

		mappings[localPort] = targetPort
	}

	return c.PodPortForward(ctx, pod.Namespace, pod.Name, address, mappings, readyChan)
}

func (c *client) PodPortForward(ctx context.Context, namespace, name, address string, ports map[int]int, readyChan chan struct{}) error {
	if address == "" {
		address = "localhost"
	}

	mappings := make([]string, 0)

	for s, t := range ports {
		mappings = append(mappings, fmt.Sprintf("%d:%d", s, t))
	}

	dialer, err := c.createyDialer(namespace, name)

	if err != nil {
		return err
	}

	forwarder, err := portforward.NewOnAddresses(dialer, []string{address}, mappings, ctx.Done(), readyChan, io.Discard, io.Discard)

	if err != nil {
		return err
	}

	return forwarder.ForwardPorts()
}

func (c *client) createyDialer(namespace, name string) (httpstream.Dialer, error) {
	primary, err := c.createPrimaryDialer(namespace, name)

	if err != nil {
		return nil, err
	}

	secondary, err := c.createSecondaryDialer(namespace, name)

	if err != nil {
		return nil, err
	}

	dialer := portforward.NewFallbackDialer(primary, secondary, func(err error) bool {
		return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
	})

	return dialer, nil
}

func (c *client) createPrimaryDialer(namespace, name string) (httpstream.Dialer, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, name)

	host := c.Config().Host
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")

	url := &url.URL{Scheme: "https", Path: path, Host: host}

	dialer, err := portforward.NewSPDYOverWebsocketDialer(url, c.Config())

	if err != nil {
		return nil, err
	}

	return dialer, nil
}

func (c *client) createSecondaryDialer(namespace, name string) (httpstream.Dialer, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, name)

	host := c.Config().Host
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")

	url := &url.URL{Scheme: "https", Path: path, Host: host}

	transport, upgrader, err := spdy.RoundTripperFor(c.Config())

	if err != nil {
		return nil, err
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, url)

	return dialer, nil
}
