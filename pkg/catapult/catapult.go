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
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

type Catapult struct {
	client  kubernetes.Client
	options CatapultOptions

	hosts *system.HostsSection

	tunnels []*tunnel

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
		if err := c.Refresh(ctx); err != nil {
			if c.options.Logger != nil {
				c.options.Logger.ErrorContext(ctx, "refresh failed", "error", err)
			}
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
	var result error

	tunnels := c.listTunnel()

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
			c.hosts.Remove(tunnel.address)

			if err := tunnel.Stop(); err != nil {
				result = errors.Join(result, err)
				continue
			}

			if c.options.DeleteFunc != nil {
				c.options.DeleteFunc(tunnel.address, tunnel.hosts, maps.Keys(tunnel.ports))
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
			if err := tunnel.Start(ctx, nil); err != nil {
				result = errors.Join(result, err)
				continue
			}

			c.hosts.Add(tunnel.address, tunnel.hosts...)

			if c.options.AddFunc != nil {
				c.options.AddFunc(tunnel.address, tunnel.hosts, maps.Keys(tunnel.ports))
			}
		}
	}

	c.tunnels = tunnels

	if err := c.hosts.Flush(); err != nil {
		return err
	}

	return nil
}

func (c *Catapult) listTunnel() []*tunnel {
	tunnels := make([]*tunnel, 0)

	for _, service := range c.services {
		if len(service.Spec.Selector) == 0 {
			continue
		}

		pods := c.selectPods(service.Namespace, labels.SelectorFromSet(service.Spec.Selector))

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

	for _, p := range list.Items {
		c.pods[resourceKey(&p)] = p
	}

	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			p := obj.(*corev1.Pod)
			c.pods[resourceKey(p)] = *p
		},

		UpdateFunc: func(oldObj, newObj interface{}) {
			p := newObj.(*corev1.Pod)
			c.pods[resourceKey(p)] = *p
		},

		DeleteFunc: func(obj interface{}) {
			p := obj.(*corev1.Pod)
			delete(c.pods, resourceKey(p))
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

func (c *Catapult) selectPods(namespace string, selector labels.Selector) []corev1.Pod {
	var result []corev1.Pod

	for _, pod := range c.pods {
		if pod.Namespace != namespace {
			continue
		}

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
