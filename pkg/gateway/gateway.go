package gateway

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/adrianliechti/loop/pkg/forward"
	"github.com/adrianliechti/loop/pkg/kubernetes"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// addressGroup is the second octet of the loopback aliases handed out here; it
// keeps gateway addresses disjoint from the catapult's.
const addressGroup = 245

type Gateway struct {
	client  kubernetes.Client
	options GatewayOptions

	registry *forward.Registry

	services   *forward.Store[corev1.Service]
	gateways   *forward.Store[gatewayv1.Gateway]
	httproutes *forward.Store[gatewayv1.HTTPRoute]
	ingresses  *forward.Store[networkingv1.Ingress]
}

type GatewayOptions struct {
	Namespaces []string

	Logger *slog.Logger

	AddFunc    func(address string, hosts []string, ports []int)
	DeleteFunc func(address string, hosts []string, ports []int)
}

func New(client kubernetes.Client, options GatewayOptions) (*Gateway, error) {
	registry, err := forward.NewRegistry("Loop Gateway", options.Logger, options.AddFunc, options.DeleteFunc)

	if err != nil {
		return nil, err
	}

	return &Gateway{
		client:  client,
		options: options,

		registry: registry,

		services:   forward.NewStore[corev1.Service](),
		gateways:   forward.NewStore[gatewayv1.Gateway](),
		httproutes: forward.NewStore[gatewayv1.HTTPRoute](),
		ingresses:  forward.NewStore[networkingv1.Ingress](),
	}, nil
}

func (c *Gateway) Start(ctx context.Context) error {
	c.registry.Reset()

	namespaces := c.options.Namespaces

	if len(namespaces) == 0 {
		namespaces = []string{""}
	}

	// Services are watched cluster-wide regardless of scope: an ingress or route
	// in one namespace routinely publishes an address belonging to a controller
	// service in another.
	if err := c.watchServices(ctx, ""); err != nil {
		return err
	}

	for _, namespace := range namespaces {
		if err := c.watchGateways(ctx, namespace); err != nil {
			return err
		}

		if err := c.watchHTTPRoutes(ctx, namespace); err != nil {
			return err
		}

		if err := c.watchIngresses(ctx, namespace); err != nil {
			return err
		}
	}

	return c.registry.Run(ctx, c.listTunnels)
}

func (c *Gateway) listTunnels(ctx context.Context) ([]*forward.Tunnel, error) {
	ingresses := c.ingresses.List()
	gateways := c.gateways.List()
	httproutes := c.httproutes.List()
	services := c.services.List()

	tunnels := make(map[string]*forward.Tunnel)

	mappings := make(map[string]string)

	for _, i := range ingresses {
		// Managed load balancers may publish only a Hostname (e.g. AWS NLB/ELB)
		// instead of an IP, so fall back to Hostname when IP is empty.
		var addr string

		for _, ing := range i.Status.LoadBalancer.Ingress {
			if ing.IP != "" {
				addr = ing.IP
				break
			}

			if ing.Hostname != "" {
				addr = ing.Hostname
			}
		}

		if addr == "" {
			continue
		}

		for _, r := range i.Spec.Rules {
			if r.Host == "" {
				continue
			}

			mappings[r.Host] = addr
		}
	}

	for _, g := range gateways {
		addr := gatewayAddress(g)

		if addr == "" {
			continue
		}

		for _, l := range g.Spec.Listeners {
			if l.Hostname == nil {
				continue
			}

			mappings[string(*l.Hostname)] = addr
		}
	}

	for _, r := range httproutes {
		var addr string

		for _, p := range r.Spec.ParentRefs {
			if p.Kind == nil || *p.Kind != "Gateway" {
				continue
			}

			for _, g := range gateways {
				if g.Namespace != r.Namespace || g.Name != string(p.Name) {
					continue
				}

				if a := gatewayAddress(g); a != "" {
					addr = a
				}
			}
		}

		if addr == "" {
			continue
		}

		for _, host := range r.Spec.Hostnames {
			mappings[string(host)] = addr
		}
	}

	for host, addr := range mappings {
		if strings.Contains(host, "*") {
			continue
		}

		if tunnel, ok := tunnels[addr]; ok {
			tunnel.AddHost(host)
			continue
		}

		service, ok := findService(services, addr)

		if !ok {
			continue
		}

		pods, err := c.client.CoreV1().Pods(service.Namespace).List(ctx, metav1.ListOptions{
			FieldSelector: "status.phase=Running",
			LabelSelector: labels.SelectorFromSet(service.Spec.Selector).String(),
		})

		if err != nil {
			continue
		}

		if len(pods.Items) == 0 {
			continue
		}

		pod := pods.Items[0]

		address := forward.MapAddress(addressGroup, service.Spec.ClusterIP)
		ports := kubernetes.ServicePortMap(*service, pod.Spec.Containers...)

		tunnels[addr] = forward.NewTunnel(c.client, pod.Namespace, pod.Name, address, ports, []string{host})
	}

	return slices.Collect(maps.Values(tunnels)), nil
}

func (c *Gateway) watchServices(ctx context.Context, namespace string) error {
	list, err := c.client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})

	if err != nil {
		return err
	}

	forward.Watch(ctx, c.services, c.client.CoreV1().RESTClient(), "services", namespace, list.Items)

	return nil
}

func (c *Gateway) watchGateways(ctx context.Context, namespace string) error {
	list, err := c.client.GatewayV1().Gateways(namespace).List(ctx, metav1.ListOptions{})

	if err != nil {
		// The Gateway API CRDs are optional; without them there is nothing to
		// watch and ingresses alone still work.
		if kubernetes.IsNotFound(err) {
			return nil
		}

		return err
	}

	forward.Watch(ctx, c.gateways, c.client.GatewayV1().RESTClient(), "gateways", namespace, list.Items)

	return nil
}

func (c *Gateway) watchHTTPRoutes(ctx context.Context, namespace string) error {
	list, err := c.client.GatewayV1().HTTPRoutes(namespace).List(ctx, metav1.ListOptions{})

	if err != nil {
		if kubernetes.IsNotFound(err) {
			return nil
		}

		return err
	}

	forward.Watch(ctx, c.httproutes, c.client.GatewayV1().RESTClient(), "httproutes", namespace, list.Items)

	return nil
}

func (c *Gateway) watchIngresses(ctx context.Context, namespace string) error {
	list, err := c.client.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})

	if err != nil {
		if kubernetes.IsNotFound(err) {
			return nil
		}

		return err
	}

	forward.Watch(ctx, c.ingresses, c.client.NetworkingV1().RESTClient(), "ingresses", namespace, list.Items)

	return nil
}

func findService(services []corev1.Service, addr string) (*corev1.Service, bool) {
	for i := range services {
		s := &services[i]

		for _, ip := range s.Spec.ClusterIPs {
			if ip == addr {
				return s, true
			}
		}

		for _, ing := range s.Status.LoadBalancer.Ingress {
			if ing.IP == addr || ing.Hostname == addr {
				return s, true
			}
		}
	}

	return nil, false
}

// gatewayAddress returns the gateway's external address, preferring IP over hostname.
func gatewayAddress(g gatewayv1.Gateway) string {
	var addr string

	for _, a := range g.Status.Addresses {
		if a.Type == nil || *a.Type == gatewayv1.HostnameAddressType {
			addr = a.Value
		}
	}

	for _, a := range g.Status.Addresses {
		if a.Type != nil && *a.Type == gatewayv1.IPAddressType {
			addr = a.Value
		}
	}

	return addr
}
