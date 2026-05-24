package catapult

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
	"sync"
	"time"

	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/system"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

type Catapult struct {
	client  kubernetes.Client
	options CatapultOptions

	hosts *system.HostsSection

	tunnels []*tunnel

	mu       sync.Mutex
	pods     map[string]corev1.Pod
	services map[string]corev1.Service
}

type CatapultOptions struct {
	Scope      string
	Namespaces []string

	Logger *slog.Logger

	AddFunc    func(address string, hosts []string, ports []int)
	DeleteFunc func(address string, hosts []string, ports []int)
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

		tunnels: make([]*tunnel, 0),

		pods:     make(map[string]corev1.Pod),
		services: make(map[string]corev1.Service),
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

	namespaces := c.options.Namespaces

	if len(namespaces) == 0 {
		namespaces = []string{""}
	}

	for _, namespace := range namespaces {
		if err := c.watchPods(ctx, c.client, namespace); err != nil {
			return err
		}

		if err := c.watchServices(ctx, c.client, namespace); err != nil {
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

func (c *Catapult) Refresh(ctx context.Context) error {
	var result error

	desired := c.listTunnel()

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

func (c *Catapult) listTunnel() []*tunnel {
	c.mu.Lock()
	allServices := slices.Collect(maps.Values(c.services))
	allPods := slices.Collect(maps.Values(c.pods))
	c.mu.Unlock()

	tunnels := make([]*tunnel, 0)

	for _, service := range allServices {
		if len(service.Spec.Selector) == 0 {
			continue
		}

		matching := selectPods(allPods, service.Namespace, labels.SelectorFromSet(service.Spec.Selector))

		if service.Spec.ClusterIP == corev1.ClusterIPNone {
			for _, pod := range matching {
				hosts := []string{
					fmt.Sprintf("%s.%s.%s.svc.cluster.local", pod.Name, service.Name, service.Namespace),
				}

				address := mapAddress(hosts[0])
				ports := selectPorts(service, pod.Spec.Containers...)

				tunnels = append(tunnels, newTunnel(c.client, pod.Namespace, pod.Name, address, ports, hosts))
			}

			continue
		}

		if len(matching) == 0 {
			continue
		}

		pod := matching[0]

		hosts := []string{
			fmt.Sprintf("%s.%s", service.Name, service.Namespace),
			fmt.Sprintf("%s.%s.svc.cluster.local", service.Name, service.Namespace),
		}

		if service.Namespace == c.options.Scope {
			hosts = append([]string{service.Name}, hosts...)
		}

		address := mapAddress(service.Spec.ClusterIP)
		ports := selectPorts(service, pod.Spec.Containers...)

		tunnels = append(tunnels, newTunnel(c.client, pod.Namespace, pod.Name, address, ports, hosts))
	}

	return tunnels
}

func resourceKey(obj metav1.Object) string {
	return fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())
}

func (c *Catapult) watchPods(ctx context.Context, client kubernetes.Client, namespace string) error {
	list, err := c.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})

	if err != nil {
		return err
	}

	c.mu.Lock()
	for _, p := range list.Items {
		c.pods[resourceKey(&p)] = p
	}
	c.mu.Unlock()

	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			p := obj.(*corev1.Pod)
			c.mu.Lock()
			c.pods[resourceKey(p)] = *p
			c.mu.Unlock()
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			p := newObj.(*corev1.Pod)
			c.mu.Lock()
			c.pods[resourceKey(p)] = *p
			c.mu.Unlock()
		},

		DeleteFunc: func(obj interface{}) {
			p := obj.(*corev1.Pod)
			c.mu.Lock()
			delete(c.pods, resourceKey(p))
			c.mu.Unlock()
		},
	}

	watcher := cache.NewListWatchFromClient(client.CoreV1().RESTClient(), "pods", namespace, fields.Everything())

	_, controller := cache.NewInformer(watcher, &corev1.Pod{}, 0, handlers)
	go controller.Run(ctx.Done())

	return nil
}

func (c *Catapult) watchServices(ctx context.Context, client kubernetes.Client, namespace string) error {
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

func selectPods(pods []corev1.Pod, namespace string, selector labels.Selector) []corev1.Pod {
	var result []corev1.Pod

	for _, pod := range pods {
		if pod.Namespace != namespace {
			continue
		}

		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		if !selector.Matches(labels.Set(pod.Labels)) {
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
