package gateway

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	"golang.org/x/exp/maps"

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

	services   map[string]corev1.Service
	gateways   map[string]gatewayv1.Gateway
	httproutes map[string]gatewayv1.HTTPRoute
	ingresses  map[string]networkingv1.Ingress
}

type GatewayOptions struct {
	Namespaces []string
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

func (c *Gateway) Refresh(ctx context.Context) error {
	var result error

	tunnels, err := c.listTunnel(ctx)

	if err != nil {
		return err
	}

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

func (c *Gateway) listTunnel(ctx context.Context) ([]*tunnel, error) {
	tunnels := make(map[string]*tunnel)

	mappings := make(map[string]string)

	for _, i := range c.ingresses {
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

	for _, g := range c.gateways {
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

	for _, r := range c.httproutes {
		addr := ""
		hosts := r.Spec.Hostnames

		for _, p := range r.Spec.ParentRefs {
			if p.Kind == nil || *p.Kind != "Gateway" {
				continue
			}

			for _, g := range c.gateways {
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
		if strings.Contains(host, "*") {
			continue
		}

		if tunnel, ok := tunnels[addr]; ok {
			tunnel.hosts = append(tunnel.hosts, host)
			continue
		}

		service, ok := c.findService(addr)

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

	return maps.Values(tunnels), nil
}

func resourceKey(obj metav1.Object) string {
	return fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())
}

func (c *Gateway) watchServices(ctx context.Context, client kubernetes.Client, namespace string) error {
	list, err := c.client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})

	if err != nil {
		return err
	}

	for _, s := range list.Items {
		c.services[resourceKey(&s)] = s
	}

	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			s := obj.(*corev1.Service)
			c.services[resourceKey(s)] = *s
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			s := newObj.(*corev1.Service)
			c.services[resourceKey(s)] = *s
		},

		DeleteFunc: func(obj interface{}) {
			s := obj.(*corev1.Service)
			delete(c.services, resourceKey(s))
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
		return err
	}

	for _, g := range list.Items {
		c.gateways[resourceKey(&g)] = g
	}

	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			g := obj.(*gatewayv1.Gateway)
			c.gateways[resourceKey(g)] = *g
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			g := newObj.(*gatewayv1.Gateway)
			c.gateways[resourceKey(g)] = *g
		},

		DeleteFunc: func(obj interface{}) {
			g := obj.(*gatewayv1.Gateway)
			delete(c.gateways, resourceKey(g))
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
		return err
	}

	for _, r := range list.Items {
		c.httproutes[resourceKey(&r)] = r
	}

	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			r := obj.(*gatewayv1.HTTPRoute)
			c.httproutes[resourceKey(r)] = *r
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			r := newObj.(*gatewayv1.HTTPRoute)
			c.httproutes[resourceKey(r)] = *r
		},

		DeleteFunc: func(obj interface{}) {
			r := obj.(*gatewayv1.HTTPRoute)
			delete(c.httproutes, resourceKey(r))
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
		return err
	}

	for _, i := range list.Items {
		c.ingresses[resourceKey(&i)] = i
	}

	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			i := obj.(*networkingv1.Ingress)
			c.ingresses[resourceKey(i)] = *i
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			i := newObj.(*networkingv1.Ingress)
			c.ingresses[resourceKey(i)] = *i
		},

		DeleteFunc: func(obj interface{}) {
			i := obj.(*networkingv1.Ingress)
			delete(c.ingresses, resourceKey(i))
		},
	}

	watcher := cache.NewListWatchFromClient(client.NetworkingV1().RESTClient(), "ingresses", namespace, fields.Everything())

	_, controller := cache.NewInformer(watcher, &networkingv1.Ingress{}, 0, handlers)
	go controller.Run(ctx.Done())

	return nil
}

func (c *Gateway) findService(addr string) (*corev1.Service, bool) {
	for _, s := range c.services {
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
