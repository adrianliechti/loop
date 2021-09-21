package kubernetes

import (
	"context"
	"errors"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (c *client) ServicePods(ctx context.Context, namespace, name string) ([]corev1.Pod, error) {
	service, err := c.CoreV1().Services(namespace).
		Get(ctx, name, metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	set := labels.Set(service.Spec.Selector)

	pods, err := c.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: set.AsSelector().String(),
	})

	return pods.Items, err
}

func (c *client) ServicePod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	pods, err := c.ServicePods(ctx, namespace, name)

	if err != nil {
		return nil, err
	}

	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodRunning {
			return &pod, nil
		}
	}

	return nil, errors.New("no running pod found")
}

func (c *client) WaitForPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	timeout := time.After(120 * time.Second)
	ticker := time.NewTicker(5 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("cancelled")
		case <-timeout:
			return nil, errors.New("timeout")
		case <-ticker.C:
			pod, err := c.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})

			if err != nil {
				continue
			}

			switch pod.Status.Phase {
			case corev1.PodRunning:
				return pod, nil
			case corev1.PodFailed, v1.PodSucceeded:
				return pod, errors.New("failed")
			}

			if pod.Status.Phase != corev1.PodRunning {
				continue
			}

			return pod, nil
		}
	}
}

func (c *client) WaitForService(ctx context.Context, namespace, name string) (*corev1.Service, error) {
	timeout := time.After(120 * time.Second)
	ticker := time.NewTicker(5 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("cancelled")
		case <-timeout:
			return nil, errors.New("timeout")
		case <-ticker.C:
			service, err := c.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})

			if err != nil {
				continue
			}

			if service.Spec.Type == corev1.ServiceTypeLoadBalancer {
				for _, ingress := range service.Status.LoadBalancer.Ingress {
					if ingress.IP != "" {
						return service, nil
					}
				}
			}
		}
	}
}
