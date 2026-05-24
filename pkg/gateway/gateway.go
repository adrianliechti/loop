package gateway

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/system"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type Gateway struct {
	client  kubernetes.Client
	options GatewayOptions

	hosts *system.HostsSection

	tunnels []*tunnel

	mu         sync.Mutex
	services   map[string]corev1.Service
	gateways   map[string]gatewayv1.Gateway
	httproutes map[string]gatewayv1.HTTPRoute
	ingresses  map[string]networkingv1.Ingress
}

type GatewayOptions struct {
	Namespaces []string

	Logger *slog.Logger

	AddFunc    func(address string, hosts []string, ports []int)
	DeleteFunc func(address string, hosts []string, ports []int)
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

		tunnels: make([]*tunnel, 0),

		services:   make(map[string]corev1.Service),
		gateways:   make(map[string]gatewayv1.Gateway),
		httproutes: make(map[string]gatewayv1.HTTPRoute),
		ingresses:  make(map[string]networkingv1.Ingress),
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

	namespaces := c.options.Namespaces

	if len(namespaces) == 0 {
		namespaces = []string{""}
	}

	if err := c.watchServices(ctx, c.client, ""); err != nil {
		return err
	}

	for _, namespace := range namespaces {
		if err := c.watchGateways(ctx, c.client, namespace); err != nil {
			return err
		}

		if err := c.watchHTTPRoutes(ctx, c.client, namespace); err != nil {
			return err
		}

		if err := c.watchIngresses(ctx, c.client, namespace); err != nil {
			return err
		}
	}

	if err := c.Refresh(ctx); err != nil {
		return err
	}

	for {
		select {
		case <-time.After(10 * time.Second):
		case <-ctx.Done():
			return nil
		}

		if err := c.Refresh(ctx); err != nil {
			if c.options.Logger != nil {
				c.options.Logger.ErrorContext(ctx, "refresh failed", "error", err)
			}
		}
	}
}

func (c *Gateway) Refresh(ctx context.Context) error {
	var result error

	desired, err := c.listTunnel(ctx)

	if err != nil {
		return err
	}

	desiredByAddr := make(map[string]*tunnel, len(desired))
	for _, t := range desired {
		desiredByAddr[t.address] = t
	}

	previousByAddr := make(map[string]*tunnel, len(c.tunnels))
	for _, t := range c.tunnels {
		previousByAddr[t.address] = t
	}

	// Stop tunnels whose address is gone, OR whose descriptor changed
	// (different hosts/ports/target pod). Anything still equivalent is reused.
	for addr, t := range previousByAddr {
		desiredT, kept := desiredByAddr[addr]

		if kept && t.equivalent(desiredT) {
			continue
		}

		c.hosts.Remove(t.address)

		if err := t.Stop(); err != nil {
			result = errors.Join(result, err)
			continue
		}

		if c.options.DeleteFunc != nil {
			c.options.DeleteFunc(t.address, t.hosts, slices.Collect(maps.Keys(t.ports)))
		}
	}

	// Build the new active tunnel set. Reuse the prior tunnel object (keeping
	// its running goroutine + cancel func) only when the descriptor is
	// equivalent; otherwise start the new one.
	next := make([]*tunnel, 0, len(desired))

	for _, t := range desired {
		if prev, ok := previousByAddr[t.address]; ok && prev.equivalent(t) {
			next = append(next, prev)
			continue
		}

		if err := t.Start(ctx, nil); err != nil {
			result = errors.Join(result, err)
			continue
		}

		c.hosts.Add(t.address, t.hosts...)

		if c.options.AddFunc != nil {
			c.options.AddFunc(t.address, t.hosts, slices.Collect(maps.Keys(t.ports)))
		}

		next = append(next, t)
	}

	c.tunnels = next

	// Do not flush if context is cancelled - let defer cleanup handle it
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if err := c.hosts.Flush(); err != nil {
		return errors.Join(result, err)
	}

	return result
}

func (c *Gateway) listTunnel(ctx context.Context) ([]*tunnel, error) {
	c.mu.Lock()
	ingresses := slices.Collect(maps.Values(c.ingresses))
	gateways := slices.Collect(maps.Values(c.gateways))
	httproutes := slices.Collect(maps.Values(c.httproutes))
	services := slices.Collect(maps.Values(c.services))
	c.mu.Unlock()

	tunnels := make(map[string]*tunnel)

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
		var hosts []string

		for _, l := range g.Spec.Listeners {
			if l.Hostname == nil {
				continue
			}

			hosts = append(hosts, string(*l.Hostname))
		}

		addr := gatewayAddress(g)

		if addr == "" {
			continue
		}

		for _, host := range hosts {
			mappings[host] = addr
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
			tunnel.hosts = append(tunnel.hosts, host)
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

		address := mapAddress(service.Spec.ClusterIP)
		ports := selectPorts(*service, pod.Spec.Containers...)

		tunnel := newTunnel(c.client, pod.Namespace, pod.Name, address, ports, []string{host})
		tunnels[addr] = tunnel
	}

	return slices.Collect(maps.Values(tunnels)), nil
}

func resourceKey(obj metav1.Object) string {
	return fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())
}

func (c *Gateway) watchServices(ctx context.Context, client kubernetes.Client, namespace string) error {
	list, err := c.client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})

	if err != nil {
		return err
	}

	c.mu.Lock()
	for _, s := range list.Items {
		c.services[resourceKey(&s)] = s
	}
	c.mu.Unlock()

	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			s := obj.(*corev1.Service)
			c.mu.Lock()
			c.services[resourceKey(s)] = *s
			c.mu.Unlock()
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			s := newObj.(*corev1.Service)
			c.mu.Lock()
			c.services[resourceKey(s)] = *s
			c.mu.Unlock()
		},

		DeleteFunc: func(obj interface{}) {
			s := obj.(*corev1.Service)
			c.mu.Lock()
			delete(c.services, resourceKey(s))
			c.mu.Unlock()
		},
	}

	watcher := cache.NewListWatchFromClient(client.CoreV1().RESTClient(), "services", namespace, fields.Everything())

	_, controller := cache.NewInformer(watcher, &corev1.Service{}, 0, handlers)
	go controller.Run(ctx.Done())

	return nil
}

func (c *Gateway) watchGateways(ctx context.Context, client kubernetes.Client, namespace string) error {
	list, err := c.client.GatewayV1().Gateways(namespace).List(ctx, metav1.ListOptions{})

	if err != nil {
		if kubernetes.IsNotFound(err) {
			return nil
		}

		return err
	}

	c.mu.Lock()
	for _, g := range list.Items {
		c.gateways[resourceKey(&g)] = g
	}
	c.mu.Unlock()

	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			g := obj.(*gatewayv1.Gateway)
			c.mu.Lock()
			c.gateways[resourceKey(g)] = *g
			c.mu.Unlock()
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			g := newObj.(*gatewayv1.Gateway)
			c.mu.Lock()
			c.gateways[resourceKey(g)] = *g
			c.mu.Unlock()
		},

		DeleteFunc: func(obj interface{}) {
			g := obj.(*gatewayv1.Gateway)
			c.mu.Lock()
			delete(c.gateways, resourceKey(g))
			c.mu.Unlock()
		},
	}

	watcher := cache.NewListWatchFromClient(client.GatewayV1().RESTClient(), "gateways", namespace, fields.Everything())

	_, controller := cache.NewInformer(watcher, &gatewayv1.Gateway{}, 0, handlers)
	go controller.Run(ctx.Done())

	return nil
}

func (c *Gateway) watchHTTPRoutes(ctx context.Context, client kubernetes.Client, namespace string) error {
	list, err := c.client.GatewayV1().HTTPRoutes(namespace).List(ctx, metav1.ListOptions{})

	if err != nil {
		if kubernetes.IsNotFound(err) {
			return nil
		}

		return err
	}

	c.mu.Lock()
	for _, r := range list.Items {
		c.httproutes[resourceKey(&r)] = r
	}
	c.mu.Unlock()

	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			r := obj.(*gatewayv1.HTTPRoute)
			c.mu.Lock()
			c.httproutes[resourceKey(r)] = *r
			c.mu.Unlock()
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			r := newObj.(*gatewayv1.HTTPRoute)
			c.mu.Lock()
			c.httproutes[resourceKey(r)] = *r
			c.mu.Unlock()
		},

		DeleteFunc: func(obj interface{}) {
			r := obj.(*gatewayv1.HTTPRoute)
			c.mu.Lock()
			delete(c.httproutes, resourceKey(r))
			c.mu.Unlock()
		},
	}

	watcher := cache.NewListWatchFromClient(client.GatewayV1().RESTClient(), "httproutes", namespace, fields.Everything())

	_, controller := cache.NewInformer(watcher, &gatewayv1.HTTPRoute{}, 0, handlers)
	go controller.Run(ctx.Done())

	return nil
}

func (c *Gateway) watchIngresses(ctx context.Context, client kubernetes.Client, namespace string) error {
	list, err := c.client.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})

	if err != nil {
		if kubernetes.IsNotFound(err) {
			return nil
		}

		return err
	}

	c.mu.Lock()
	for _, i := range list.Items {
		c.ingresses[resourceKey(&i)] = i
	}
	c.mu.Unlock()

	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			i := obj.(*networkingv1.Ingress)
			c.mu.Lock()
			c.ingresses[resourceKey(i)] = *i
			c.mu.Unlock()
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			i := newObj.(*networkingv1.Ingress)
			c.mu.Lock()
			c.ingresses[resourceKey(i)] = *i
			c.mu.Unlock()
		},

		DeleteFunc: func(obj interface{}) {
			i := obj.(*networkingv1.Ingress)
			c.mu.Lock()
			delete(c.ingresses, resourceKey(i))
			c.mu.Unlock()
		},
	}

	watcher := cache.NewListWatchFromClient(client.NetworkingV1().RESTClient(), "ingresses", namespace, fields.Everything())

	_, controller := cache.NewInformer(watcher, &networkingv1.Ingress{}, 0, handlers)
	go controller.Run(ctx.Done())

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
	addr[1] = 245

	ip := net.IP(addr)
	return ip.String()
}
