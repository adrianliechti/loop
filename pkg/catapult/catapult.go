package catapult

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/system"

	"golang.org/x/exp/maps"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type Catapult struct {
	client  kubernetes.Client
	options CatapultOptions

	tunnels []*tunnel
	hosts   *system.HostsSection
}

type CatapultOptions struct {
	Scope      string
	Namespaces []string

	Selector string
}

func New(client kubernetes.Client, options CatapultOptions) (*Catapult, error) {
	hosts, err := system.NewHostsSection("Loop Catapult")

	if err != nil {
		return nil, err
	}

	return &Catapult{
		client:  client,
		options: options,

		hosts: hosts,
	}, nil
}

func (c *Catapult) Start(ctx context.Context) error {
	c.hosts.Clear()
	c.hosts.Flush()

	defer func() {
		c.hosts.Clear()
		c.hosts.Flush()

		for _, t := range c.tunnels {
			t.Stop()
		}
	}()

	for {
		if err := c.Refresh(ctx); err != nil {
			slog.ErrorContext(ctx, "refresh failed", "error", err)
		}

		select {
		case <-time.After(10 * time.Second):
			continue

		case <-ctx.Done():
			return nil
		}
	}
}

func (c *Catapult) Refresh(ctx context.Context) error {
	tunnels, err := c.listTunnel(ctx)

	if err != nil {
		return err
	}

	var result error

	// remove unused tunnels
	for _, i := range c.tunnels {
		tunnel := i
		removed := true

		for _, t := range tunnels {
			if tunnel.address == t.address {
				removed = false
				break
			}
		}

		if removed {
			slog.InfoContext(ctx, "removing tunnel", "namespace", tunnel.namespace, "hosts", tunnel.hosts, "ports", maps.Keys(tunnel.ports))

			c.hosts.Remove(tunnel.address)

			if err := tunnel.Stop(); err != nil {
				result = errors.Join(result, err)
				continue
			}
		}
	}

	// add new tunnels
	for _, i := range tunnels {
		tunnel := i
		added := true

		for _, t := range c.tunnels {
			if tunnel.address == t.address {
				added = false
				break
			}
		}

		if added {
			slog.InfoContext(ctx, "adding tunnel", "namespace", tunnel.namespace, "hosts", tunnel.hosts, "ports", maps.Keys(tunnel.ports))

			if err := tunnel.Start(ctx, nil); err != nil {
				result = errors.Join(result, err)
				continue
			}

			c.hosts.Add(tunnel.address, tunnel.hosts...)
		}
	}

	c.tunnels = tunnels

	if err := c.hosts.Flush(); err != nil {
		return err
	}

	return nil
}

func (c *Catapult) listTunnel(ctx context.Context) ([]*tunnel, error) {
	tunnels := make([]*tunnel, 0)

	services, err := c.listServices(ctx)

	if err != nil {
		return tunnels, err
	}

	pods, err := c.listPods(ctx)

	if err != nil {
		return tunnels, err
	}

	for _, service := range services {
		if len(service.Spec.Selector) == 0 {
			continue
		}

		pods := selectPods(pods, labels.SelectorFromSet(service.Spec.Selector))

		if service.Spec.ClusterIP == corev1.ClusterIPNone {
			for _, pod := range pods {
				hosts := []string{
					fmt.Sprintf("%s.%s.%s.svc.cluster.local", pod.Name, service.Name, service.Namespace),
				}

				address := mapAddress(hosts[0])
				ports := selectPorts(service, pod.Spec.Containers...)

				tunnels = append(tunnels, newTunnel(c.client, pod.Namespace, pod.Name, address, ports, hosts))
			}
		} else {
			if len(pods) > 0 {
				pod := pods[0]

				hosts := []string{
					fmt.Sprintf("%s.%s", service.Name, service.Namespace),
					fmt.Sprintf("%s.%s.svc.cluster.local", service.Name, service.Namespace),
				}

				address := mapAddress(service.Spec.ClusterIP)
				ports := selectPorts(service, pod.Spec.Containers...)

				if service.Namespace == c.options.Scope {
					hosts = append([]string{service.Name}, hosts...)
				}

				tunnels = append(tunnels, newTunnel(c.client, pod.Namespace, pod.Name, address, ports, hosts))
			}
		}
	}

	return tunnels, nil
}

func (c *Catapult) listServices(ctx context.Context) ([]corev1.Service, error) {
	var result []corev1.Service

	if len(c.options.Namespaces) == 0 {
		list, err := c.client.CoreV1().Services("").List(ctx, metav1.ListOptions{
			LabelSelector: c.options.Selector,
		})

		if err != nil {
			return nil, err
		}

		return list.Items, nil
	}

	for _, namespace := range c.options.Namespaces {
		list, err := c.client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: c.options.Selector,
		})

		if err != nil {
			return nil, err
		}

		result = append(result, list.Items...)
	}

	return result, nil
}

func (c *Catapult) listPods(ctx context.Context) ([]corev1.Pod, error) {
	var result []corev1.Pod

	if len(c.options.Namespaces) == 0 {
		list, err := c.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{
			LabelSelector: c.options.Selector,
		})

		if err != nil {
			return nil, err
		}

		return list.Items, nil
	}

	for _, namespace := range c.options.Namespaces {
		list, err := c.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: c.options.Selector,
		})

		if err != nil {
			return nil, err
		}

		result = append(result, list.Items...)
	}

	return result, nil
}

func selectPods(pods []corev1.Pod, selector labels.Selector) []corev1.Pod {
	var result []corev1.Pod

	for _, pod := range pods {
		// skip non-runing pods
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		labels := labels.Set(pod.Labels)

		// filter pods by selector
		if !selector.Matches(labels) {
			continue
		}

		result = append(result, pod)
	}

	return result
}

func selectPorts(service corev1.Service, containers ...corev1.Container) map[int]int {
	ports := make(map[int]int)

	for _, port := range service.Spec.Ports {
		servicePort := int(port.Port)
		containerPort := 0

		if port.Protocol != "" && port.Protocol != corev1.ProtocolTCP {
			continue
		}

		for _, c := range containers {
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
			ports[servicePort] = containerPort
		}
	}

	return ports
}

func mapAddress(address string) string {
	h := md5.New()
	io.WriteString(h, address)

	addr := h.Sum(nil)

	addr = addr[:4]
	addr[0] = 127
	addr[1] = 244

	ip := net.IP(addr)
	return ip.String()
}
