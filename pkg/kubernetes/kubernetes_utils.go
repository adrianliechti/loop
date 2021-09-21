package kubernetes

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"
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
