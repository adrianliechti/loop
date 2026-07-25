package catapult

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/adrianliechti/loop/pkg/forward"
	"github.com/adrianliechti/loop/pkg/kubernetes"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// addressGroup is the second octet of the loopback aliases handed out here; it
// keeps catapult addresses disjoint from the gateway's.
const addressGroup = 244

type Catapult struct {
	client  kubernetes.Client
	options CatapultOptions

	registry *forward.Registry

	pods     *forward.Store[corev1.Pod]
	services *forward.Store[corev1.Service]
}

type CatapultOptions struct {
	Scope      string
	Namespaces []string

	Logger *slog.Logger

	AddFunc    func(address string, hosts []string, ports []int)
	DeleteFunc func(address string, hosts []string, ports []int)
}

func New(client kubernetes.Client, options CatapultOptions) (*Catapult, error) {
	registry, err := forward.NewRegistry("Loop Catapult", options.Logger, options.AddFunc, options.DeleteFunc)

	if err != nil {
		return nil, err
	}

	return &Catapult{
		client:  client,
		options: options,

		registry: registry,

		pods:     forward.NewStore[corev1.Pod](),
		services: forward.NewStore[corev1.Service](),
	}, nil
}

func (c *Catapult) Start(ctx context.Context) error {
	c.registry.Reset()

	namespaces := c.options.Namespaces

	if len(namespaces) == 0 {
		namespaces = []string{""}
	}

	for _, namespace := range namespaces {
		if err := c.watchPods(ctx, namespace); err != nil {
			return err
		}

		if err := c.watchServices(ctx, namespace); err != nil {
			return err
		}
	}

	return c.registry.Run(ctx, c.listTunnels)
}

func (c *Catapult) listTunnels(context.Context) ([]*forward.Tunnel, error) {
	allServices := c.services.List()
	allPods := c.pods.List()

	tunnels := make([]*forward.Tunnel, 0)

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

				address := forward.MapAddress(addressGroup, hosts[0])
				ports := kubernetes.ServicePortMap(service, pod.Spec.Containers...)

				tunnels = append(tunnels, forward.NewTunnel(c.client, pod.Namespace, pod.Name, address, ports, hosts))
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

		address := forward.MapAddress(addressGroup, service.Spec.ClusterIP)
		ports := kubernetes.ServicePortMap(service, pod.Spec.Containers...)

		tunnels = append(tunnels, forward.NewTunnel(c.client, pod.Namespace, pod.Name, address, ports, hosts))
	}

	return tunnels, nil
}

func (c *Catapult) watchPods(ctx context.Context, namespace string) error {
	list, err := c.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})

	if err != nil {
		return err
	}

	forward.Watch(ctx, c.pods, c.client.CoreV1().RESTClient(), "pods", namespace, list.Items)

	return nil
}

func (c *Catapult) watchServices(ctx context.Context, namespace string) error {
	list, err := c.client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})

	if err != nil {
		return err
	}

	forward.Watch(ctx, c.services, c.client.CoreV1().RESTClient(), "services", namespace, list.Items)

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
