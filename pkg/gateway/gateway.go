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

// addressGroup is the loopback partition the aliases handed out here come from;
// it keeps gateway addresses disjoint from the catapult's.
const addressGroup = 1

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

	// Services and gateways are watched cluster-wide regardless of scope: an
	// ingress or route in one namespace routinely publishes an address belonging
	// to a controller service in another, and the usual Gateway API layout keeps
	// one shared Gateway in an infrastructure namespace with the routes that
	// attach to it spread across app namespaces. Scope is instead applied where
	// a gateway is used as a hostname source in its own right, in listTunnels.
	if err := c.watchServices(ctx, ""); err != nil {
		return err
	}

	if err := c.watchGateways(ctx, ""); err != nil {
		return err
	}

	for _, namespace := range namespaces {
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
		// Gateways are stored cluster-wide so routes can attach across
		// namespaces, but a listener hostname is only published on its own
		// account when the gateway itself is in scope.
		if !c.inScope(g.Namespace) {
			continue
		}

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
			// Per Gateway API defaulting: an unset Group means the Gateway API
			// group, an unset Kind means Gateway, and an unset Namespace means
			// the route's own namespace.
			if p.Group != nil && *p.Group != gatewayv1.GroupName {
				continue
			}

			if p.Kind != nil && *p.Kind != "Gateway" {
				continue
			}

			namespace := r.Namespace

			if p.Namespace != nil {
				namespace = string(*p.Namespace)
			}

			if !parentAccepted(r, namespace, string(p.Name)) {
				continue
			}

			for _, g := range gateways {
				if g.Namespace != namespace || g.Name != string(p.Name) {
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

	// Sorted so the hostname that names a tunnel, and the order the rest are
	// appended in, do not shift between refreshes.
	for _, host := range slices.Sorted(maps.Keys(mappings)) {
		addr := mappings[host]

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

		pod, ok := kubernetes.SelectPod(pods.Items)

		if !ok {
			continue
		}

		address := forward.MapAddress(addressGroup, service.Spec.ClusterIP)
		ports := kubernetes.ServicePortMap(*service, pod.Spec.Containers...)

		tunnels[addr] = forward.NewTunnel(c.client, pod.Namespace, pod.Name, address, ports, []string{host})
	}

	result := make([]*forward.Tunnel, 0, len(tunnels))

	for _, addr := range slices.Sorted(maps.Keys(tunnels)) {
		result = append(result, tunnels[addr])
	}

	return result, nil
}

// inScope reports whether a namespace is one the user asked to watch.
func (c *Gateway) inScope(namespace string) bool {
	if len(c.options.Namespaces) == 0 {
		return true
	}

	return slices.Contains(c.options.Namespaces, namespace)
}

// parentAccepted reports whether the gateway took the route on. Attachment is
// not the route's to declare: a listener admits routes from other namespaces
// only if its allowedRoutes says so, and admits a route at all only if their
// hostnames intersect — so a parentRef alone does not mean the gateway serves
// those hostnames, and tunneling them would point the hosts file at a gateway
// that answers for something else.
//
// A missing status is not read as a refusal. Not every implementation fills it
// in, and a Gateway that no controller has claimed publishes no address to
// tunnel to in the first place.
func parentAccepted(r gatewayv1.HTTPRoute, namespace, name string) bool {
	var rejected bool

	for _, s := range r.Status.Parents {
		if string(s.ParentRef.Name) != name {
			continue
		}

		if string(kubernetes.Deref(s.ParentRef.Namespace, gatewayv1.Namespace(r.Namespace))) != namespace {
			continue
		}

		for _, cond := range s.Conditions {
			if cond.Type != string(gatewayv1.RouteConditionAccepted) {
				continue
			}

			// One accepting listener is enough; a route may name the same
			// gateway several times over different sections.
			if cond.Status == metav1.ConditionTrue {
				return true
			}

			rejected = true
		}
	}

	return !rejected
}

func (c *Gateway) watchServices(ctx context.Context, namespace string) error {
	// Listed up front only so a permission error or a missing CRD fails Start
	// with a useful message; the stores themselves are filled by the informers.
	if _, err := c.client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{}); err != nil {
		return err
	}

	forward.Watch(ctx, c.services, c.client.CoreV1().RESTClient(), "services", namespace)

	return nil
}

func (c *Gateway) watchGateways(ctx context.Context, namespace string) error {
	if _, err := c.client.GatewayV1().Gateways(namespace).List(ctx, metav1.ListOptions{}); err != nil {
		// The Gateway API CRDs are optional; without them there is nothing to
		// watch and ingresses alone still work.
		if kubernetes.IsNotFound(err) {
			return nil
		}

		return err
	}

	forward.Watch(ctx, c.gateways, c.client.GatewayV1().RESTClient(), "gateways", namespace)

	return nil
}

func (c *Gateway) watchHTTPRoutes(ctx context.Context, namespace string) error {
	if _, err := c.client.GatewayV1().HTTPRoutes(namespace).List(ctx, metav1.ListOptions{}); err != nil {
		if kubernetes.IsNotFound(err) {
			return nil
		}

		return err
	}

	forward.Watch(ctx, c.httproutes, c.client.GatewayV1().RESTClient(), "httproutes", namespace)

	return nil
}

func (c *Gateway) watchIngresses(ctx context.Context, namespace string) error {
	if _, err := c.client.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{}); err != nil {
		if kubernetes.IsNotFound(err) {
			return nil
		}

		return err
	}

	forward.Watch(ctx, c.ingresses, c.client.NetworkingV1().RESTClient(), "ingresses", namespace)

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

// gatewayAddress returns the gateway's external address, preferring IP over
// hostname. An unset Type counts as an IP: the field defaults to IPAddress.
func gatewayAddress(g gatewayv1.Gateway) string {
	var addr string

	for _, a := range g.Status.Addresses {
		if a.Type != nil && *a.Type == gatewayv1.HostnameAddressType {
			addr = a.Value
		}
	}

	for _, a := range g.Status.Addresses {
		if a.Type == nil || *a.Type == gatewayv1.IPAddressType {
			addr = a.Value
		}
	}

	return addr
}
