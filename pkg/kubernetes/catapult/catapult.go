package catapult

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/system"

	"github.com/ChrisWiegman/goodhosts/v4/pkg/goodhosts"
	"github.com/hashicorp/go-multierror"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type Catapult struct {
	client  kubernetes.Client
	options CatapultOptions

	tunnels   []*tunnel
	hostsfile goodhosts.Hosts
}

type CatapultOptions struct {
	Scope     string
	Namespace string

	Selector string
}

func New(client kubernetes.Client, options CatapultOptions) (*Catapult, error) {
	hostsfile, err := goodhosts.NewHosts("Loop")

	if err != nil {
		return nil, err
	}

	return &Catapult{
		client:  client,
		options: options,

		hostsfile: hostsfile,
	}, nil
}

func (c *Catapult) Start(ctx context.Context) error {
	defer func() {
		c.hostsfile.Load()

		for _, t := range c.tunnels {
			system.UnaliasIP(context.Background(), t.address)
			c.hostsfile.Remove(t.address, t.hosts...)
		}

		c.hostsfile.RemoveSection()
		c.hostsfile.Flush()
	}()

	for {
		if err := ctx.Err(); err != nil {
			break
		}

		if err := c.Refresh(ctx); err != nil {
			slog.ErrorCtx(ctx, "refresh failed", "error", err)
		}

		time.Sleep(10 * time.Second)
	}

	return nil
}

func (c *Catapult) Refresh(ctx context.Context) error {
	tunnels, err := c.listTunnel(ctx)

	if err != nil {
		return err
	}

	if err := c.hostsfile.Load(); err != nil {
		return err
	}

	var result error

	// remove unused tunnels
	for _, i := range c.tunnels {
		tunnel := i
		removed := true

		for _, r := range tunnels {
			if tunnel.namespace == r.namespace && tunnel.name == r.name {
				removed = false
				break
			}
		}

		if removed {
			slog.InfoCtx(ctx, "removing tunnel", "namespace", tunnel.namespace, "hosts", tunnel.hosts, "ports", maps.Keys(tunnel.ports))

			tunnel.Stop()

			if err := system.UnaliasIP(ctx, tunnel.address); err != nil {
				result = multierror.Append(result, err)
				continue
			}

			if err := c.hostsfile.Remove(tunnel.address, tunnel.hosts...); err != nil {
				result = multierror.Append(result, err)
				continue
			}
		}
	}

	// add new tunnels
	for _, i := range tunnels {
		tunnel := i
		added := true

		for _, r := range c.tunnels {
			if tunnel.namespace == r.namespace && tunnel.name == r.name {
				added = false
				break
			}
		}

		if added {
			slog.InfoCtx(ctx, "adding tunnel", "namespace", tunnel.namespace, "hosts", tunnel.hosts, "ports", maps.Keys(tunnel.ports))

			if err := system.AliasIP(ctx, tunnel.address); err != nil {
				result = multierror.Append(result, err)
				continue
			}

			if err := c.hostsfile.Add(tunnel.address, "", tunnel.hosts...); err != nil {
				result = multierror.Append(result, err)
				continue
			}

			if err := tunnel.Start(ctx, nil); err != nil {
				result = multierror.Append(result, err)
				continue
			}
		}
	}

	c.tunnels = tunnels

	if err := c.hostsfile.Flush(); err != nil {
		return err
	}

	return nil
}

func (c *Catapult) listTunnel(ctx context.Context) ([]*tunnel, error) {
	tunnels := make([]*tunnel, 0)

	services, err := c.client.CoreV1().Services(c.options.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: c.options.Selector,
	})

	if err != nil {
		return tunnels, err
	}

	pods, err := c.client.CoreV1().Pods(c.options.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: c.options.Selector,
	})

	if err != nil {
		return tunnels, err
	}

	for _, service := range services.Items {
		if len(service.Spec.Selector) == 0 {
			continue
		}

		selector := labels.SelectorFromSet(service.Spec.Selector)

		pods := selectPods(pods.Items, selector)

		if service.Spec.ClusterIP != corev1.ClusterIPNone {
			// Normal Services

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
		} else {
			// Headless Services
			for _, pod := range pods {
				hosts := []string{
					fmt.Sprintf("%s.%s.%s.svc.cluster.local", pod.Name, service.Name, service.Namespace),
				}

				address := mapAddress(hosts[0])
				ports := selectPorts(service, pod.Spec.Containers...)

				tunnels = append(tunnels, newTunnel(c.client, pod.Namespace, pod.Name, address, ports, hosts))
			}
		}
	}

	return tunnels, nil
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
