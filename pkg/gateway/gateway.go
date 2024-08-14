package gateway

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/system"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type Gateway struct {
	client  kubernetes.Client
	options GatewayOptions

	tunnels []*tunnel
	hosts   *system.HostsSection
}

type GatewayOptions struct {
	Namespaces []string

	Selector string
}

func New(client kubernetes.Client, options GatewayOptions) (*Gateway, error) {
	hosts, err := system.NewHostsSection("Loop Gateway")

	if err != nil {
		return nil, err
	}

	return &Gateway{
		client:  client,
		options: options,

		hosts: hosts,
	}, nil
}

func (c *Gateway) Start(ctx context.Context) error {
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
		if err := ctx.Err(); err != nil {
			break
		}

		if err := c.Refresh(ctx); err != nil {
			slog.ErrorContext(ctx, "refresh failed", "error", err)
		}

		time.Sleep(10 * time.Second)
	}

	return nil
}

func (c *Gateway) Refresh(ctx context.Context) error {
	services, err := c.listServices(ctx)

	if err != nil {
		return err
	}

	gateways, err := c.listGateways(ctx)

	if err != nil {
		return err
	}

	httproutes, err := c.listHTTPRoutes(ctx)

	if err != nil {
		return err
	}

	ingresses, err := c.listIngresses(ctx)

	if err != nil {
		return err
	}

	mappings := make(map[string]string)

	for _, i := range ingresses {
		if len(i.Status.LoadBalancer.Ingress) == 0 {
			continue
		}

		ip := i.Status.LoadBalancer.Ingress[0].IP

		for _, r := range i.Spec.Rules {
			if r.Host == "" {
				continue
			}

			mappings[r.Host] = ip
		}
	}

	for _, g := range gateways {
		var addr string
		var hosts []string

		for _, l := range g.Spec.Listeners {
			if l.Hostname == nil {
				continue
			}

			hosts = append(hosts, string(*l.Hostname))
		}

		for _, a := range g.Status.Addresses {
			if a.Type == nil || *a.Type != gatewayv1.HostnameAddressType {
				addr = a.Value
			}
		}

		for _, a := range g.Status.Addresses {
			if a.Type == nil || *a.Type != gatewayv1.IPAddressType {
				addr = a.Value
			}
		}

		if addr == "" {
			continue
		}

		for _, host := range hosts {
			mappings[string(host)] = addr
		}
	}

	for _, r := range httproutes {
		addr := ""
		hosts := r.Spec.Hostnames

		for _, p := range r.Spec.ParentRefs {
			if p.Kind == nil || *p.Kind != "Gateway" {
				continue
			}

			for _, g := range gateways {
				if g.Namespace != r.Namespace || g.Name != string(p.Name) {
					continue
				}

				for _, a := range g.Status.Addresses {
					if a.Type == nil || *a.Type != gatewayv1.HostnameAddressType {
						addr = a.Value
					}
				}

				for _, a := range g.Status.Addresses {
					if a.Type == nil || *a.Type != gatewayv1.IPAddressType {
						addr = a.Value
					}
				}
			}
		}

		if addr == "" {
			continue
		}

		for _, host := range hosts {
			mappings[string(host)] = addr
		}
	}

	for host, addr := range mappings {
		service, ok := findService(services, addr)

		if !ok {
			continue
		}

		pods, err := c.client.CoreV1().Pods(service.Namespace).List(ctx, metav1.ListOptions{
			FieldSelector: "status.phase=Running",
			LabelSelector: labels.SelectorFromSet(service.Spec.Selector).String(),
		})

		if err != nil {
			return err
		}

		if len(pods.Items) == 0 {
			return errors.New("no running pods found for service")
		}

		pod := pods.Items[0]

		println(host, service.Namespace, service.Name, pod.Name)
	}

	return nil
}

type gateways struct {
}

func (c *Gateway) listServices(ctx context.Context) ([]corev1.Service, error) {
	list, err := c.client.CoreV1().Services("").List(ctx, metav1.ListOptions{
		LabelSelector: c.options.Selector,
	})

	if err != nil {
		return nil, err
	}

	return list.Items, nil
}

func findService(services []corev1.Service, addr string) (*corev1.Service, bool) {
	for _, s := range services {
		service := s

		for _, ip := range s.Spec.ClusterIPs {
			if ip == addr {
				return &service, true
			}
		}

		for _, i := range s.Status.LoadBalancer.Ingress {
			if i.IP == addr || i.Hostname == addr {
				return &service, true
			}
		}
	}

	return nil, false
}

func (c *Gateway) listIngresses(ctx context.Context) ([]networkingv1.Ingress, error) {
	if len(c.options.Namespaces) == 0 {
		list, err := c.client.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{
			LabelSelector: c.options.Selector,
		})

		if err != nil {
			if kubernetes.IsNotFound(err) {
				return []networkingv1.Ingress{}, nil
			}

			return nil, err
		}

		return list.Items, nil
	}

	var result []networkingv1.Ingress

	for _, namespace := range c.options.Namespaces {
		list, err := c.client.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: c.options.Selector,
		})

		if err != nil {
			continue
		}

		result = append(result, list.Items...)
	}

	return result, nil
}

func (c *Gateway) listGateways(ctx context.Context) ([]gatewayv1.Gateway, error) {
	if len(c.options.Namespaces) == 0 {
		list, err := c.client.GatewayV1().Gateways("").List(ctx, metav1.ListOptions{
			LabelSelector: c.options.Selector,
		})

		if err != nil {
			if kubernetes.IsNotFound(err) {
				return []gatewayv1.Gateway{}, nil
			}

			return nil, err
		}

		return list.Items, nil
	}

	var result []gatewayv1.Gateway

	for _, namespace := range c.options.Namespaces {
		list, err := c.client.GatewayV1().Gateways(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: c.options.Selector,
		})

		if err != nil {
			if kubernetes.IsNotFound(err) {
				return []gatewayv1.Gateway{}, nil
			}

			return nil, err
		}

		result = append(result, list.Items...)
	}

	return result, nil
}

func (c *Gateway) listHTTPRoutes(ctx context.Context) ([]gatewayv1.HTTPRoute, error) {
	if len(c.options.Namespaces) == 0 {
		list, err := c.client.GatewayV1().HTTPRoutes("").List(ctx, metav1.ListOptions{
			LabelSelector: c.options.Selector,
		})

		if err != nil {
			if kubernetes.IsNotFound(err) {
				return []gatewayv1.HTTPRoute{}, nil
			}

			return nil, err
		}

		return list.Items, nil
	}

	var result []gatewayv1.HTTPRoute

	for _, namespace := range c.options.Namespaces {
		list, err := c.client.GatewayV1().HTTPRoutes(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: c.options.Selector,
		})

		if err != nil {
			if kubernetes.IsNotFound(err) {
				return []gatewayv1.HTTPRoute{}, nil
			}

			return nil, err
		}

		result = append(result, list.Items...)
	}

	return result, nil
}
