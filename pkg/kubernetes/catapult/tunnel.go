package catapult

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/adrianliechti/loop/pkg/kubernetes"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type CatapultTunnel struct {
	client  kubernetes.Client
	options CatapultOptions
	service corev1.Service

	pod corev1.Pod

	addresses []string
}

func NewTunnel(client kubernetes.Client, options CatapultOptions, service corev1.Service, pod corev1.Pod) *CatapultTunnel {
	return &CatapultTunnel{
		client:  client,
		options: options,
		service: service,

		pod: pod,
	}
}

func (t *CatapultTunnel) Start(ctx context.Context, readyChan chan struct{}) error {
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", t.pod.Namespace, t.pod.Name)

	host := t.client.Config().Host
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")

	transport, upgrader, err := spdy.RoundTripperFor(t.client.Config())

	if err != nil {
		return err
	}

	address, err := t.Address()

	if err != nil {
		return err
	}

	mappings := make([]string, 0)

	for s, t := range t.Ports() {
		mappings = append(mappings, fmt.Sprintf("%s:%s", s, t))
	}

	if len(mappings) == 0 {
		return errors.New("no ports to forward")
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, &url.URL{Scheme: "https", Path: path, Host: host})
	forwarder, err := portforward.NewOnAddresses(dialer, []string{address}, mappings, ctx.Done(), readyChan, ioutil.Discard, ioutil.Discard)

	if err != nil {
		return err
	}

	go func() {
		if err := forwarder.ForwardPorts(); err != nil {
			println(err)
		}
	}()

	return nil
}

func (t *CatapultTunnel) Address() (string, error) {
	if t.service.Spec.ClusterIP == corev1.ClusterIPNone {
		return mapAddress(t.pod.Status.PodIP)
	}

	return mapAddress(t.service.Spec.ClusterIP)
}

func (t *CatapultTunnel) Hosts() []string {
	if t.service.Spec.ClusterIP != corev1.ClusterIPNone {
		// Normal Services
		hosts := []string{
			fmt.Sprintf("%s.%s.svc.cluster.local", t.service.Name, t.service.Namespace),
			fmt.Sprintf("%s.%s", t.service.Name, t.service.Namespace),
		}

		if t.service.Namespace == t.options.Scope {
			hosts = append(hosts, t.service.Name)
		}

		return hosts
	} else {
		// Headless Services
		return []string{
			fmt.Sprintf("%s.%s.%s.svc.cluster.local", t.pod.Name, t.service.Name, t.service.Namespace),
		}
	}
}

func (t *CatapultTunnel) Ports() map[string]string {
	ports := make(map[string]string)

	for _, port := range t.service.Spec.Ports {
		servicePort := int(port.Port)
		containerPort := 0

		if port.Protocol != "" && port.Protocol != corev1.ProtocolTCP {
			continue
		}

		for _, container := range t.pod.Spec.Containers {
			for _, p := range container.Ports {
				if p.Name != "" && p.Name == port.TargetPort.String() {
					containerPort = int(p.ContainerPort)
				}
			}
		}

		if port.TargetPort.IntVal > 0 {
			containerPort = int(port.TargetPort.IntVal)
		}

		if servicePort > 0 && containerPort > 0 {
			ports[strconv.Itoa(servicePort)] = strconv.Itoa(containerPort)
		}
	}

	return ports
}

func (t *CatapultTunnel) forward(ctx context.Context, readyChan chan struct{}) error {
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", t.pod.Namespace, t.pod.Name)

	host := t.client.Config().Host
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")

	transport, upgrader, err := spdy.RoundTripperFor(t.client.Config())

	if err != nil {
		return err
	}

	address, err := t.Address()

	if err != nil {
		return err
	}

	mappings := make([]string, 0)

	for s, t := range t.Ports() {
		mappings = append(mappings, fmt.Sprintf("%s:%s", s, t))
	}

	if len(mappings) == 0 {
		return errors.New("no ports to forward")
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, &url.URL{Scheme: "https", Path: path, Host: host})
	forwarder, err := portforward.NewOnAddresses(dialer, []string{address}, mappings, ctx.Done(), readyChan, ioutil.Discard, ioutil.Discard)

	if err != nil {
		return err
	}

	return forwarder.ForwardPorts()
}

func mapAddress(ip string) (string, error) {
	parts := strings.Split(ip, ".")

	if len(parts) != 4 {
		return "", errors.New("invalid pod ip")
	}

	parts[0] = "127"
	return strings.Join(parts, "."), nil
}
