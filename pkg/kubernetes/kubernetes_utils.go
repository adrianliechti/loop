package kubernetes

import (
	"context"
	"errors"
	"time"

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

func (c *client) ServiceAddress(ctx context.Context, namespace, name string) (string, error) {
	service, err := c.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})

	if err != nil {
		return "", err
	}

	if service.Spec.Type == corev1.ServiceTypeLoadBalancer {
		for _, ingress := range service.Status.LoadBalancer.Ingress {
			if ingress.IP != "" {
				return ingress.IP, nil
			}
		}
	}

	return service.Spec.ClusterIP, nil
}

func (c *client) WaitForPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	timeout := time.After(300 * time.Second)
	ticker := time.NewTicker(5 * time.Second)

LOOP:
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

			if pod.Status.Phase == corev1.PodFailed {
				return pod, errors.New("pod failed")
			}

			if pod.Status.Phase == corev1.PodSucceeded {
				return pod, errors.New("pod succeeded")
			}

			if pod.Status.Phase != corev1.PodRunning {
				continue
			}

			for _, status := range pod.Status.ContainerStatuses {
				if !status.Ready {
					continue LOOP
				}
			}

			return pod, nil
		}
	}
}

// func (c *client) WaitForPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
// 	selector := fields.OneTermEqualSelector("metadata.name", name).String()

// 	lw := &cache.ListWatch{
// 		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
// 			options.FieldSelector = selector
// 			return c.CoreV1().Pods(namespace).List(ctx, options)
// 		},

// 		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
// 			options.FieldSelector = selector
// 			return c.CoreV1().Pods(namespace).Watch(ctx, options)
// 		},
// 	}

// 	ev, err := watchtools.UntilWithSync(ctx, lw, &corev1.Pod{}, nil, podReady)

// 	if err != nil {
// 		return nil, err
// 	}

// 	return ev.Object.(*corev1.Pod), nil
// }

// func podReady(event watch.Event) (bool, error) {
// 	switch event.Type {
// 	case watch.Deleted:
// 		return false, errors.New("pod deleted")
// 	}
// 	switch t := event.Object.(type) {
// 	case *corev1.Pod:
// 		switch t.Status.Phase {
// 		case corev1.PodFailed, corev1.PodSucceeded:
// 			return false, errors.New("pod completed")

// 		case corev1.PodRunning:
// 			conditions := t.Status.Conditions

// 			if conditions == nil {
// 				return false, nil
// 			}

// 			for i := range conditions {
// 				if conditions[i].Type == corev1.PodReady &&
// 					conditions[i].Status == corev1.ConditionTrue {
// 					return true, nil
// 				}
// 			}
// 		}
// 	}

// 	return false, nil
// }

func (c *client) WaitForService(ctx context.Context, namespace, name string) (*corev1.Service, error) {
	timeout := time.After(300 * time.Second)
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
				if len(service.Status.LoadBalancer.Ingress) == 0 {
					continue
				}

				ingress := service.Status.LoadBalancer.Ingress[0]

				if ingress.IP == "" {
					continue
				}
			}

			return service, nil
		}
	}
}

// func (c *client) WaitForService(ctx context.Context, namespace, name string) (*corev1.Service, error) {
// 	selector := fields.OneTermEqualSelector("metadata.name", name).String()

// 	lw := &cache.ListWatch{
// 		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
// 			options.FieldSelector = selector
// 			return c.CoreV1().Services(namespace).List(ctx, options)
// 		},

// 		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
// 			options.FieldSelector = selector
// 			return c.CoreV1().Services(namespace).Watch(ctx, options)
// 		},
// 	}

// 	ev, err := watchtools.UntilWithSync(ctx, lw, &corev1.Service{}, nil, serviceReady)

// 	if err != nil {
// 		return nil, err
// 	}

// 	return ev.Object.(*corev1.Service), nil
// }

// func serviceReady(event watch.Event) (bool, error) {
// 	switch event.Type {
// 	case watch.Deleted:
// 		return false, errors.New("service deleted")
// 	}
// 	switch t := event.Object.(type) {
// 	case *corev1.Service:
// 		conditions := t.Status.Conditions

// 		if t.Spec.Type == corev1.ServiceTypeLoadBalancer {
// 			if len(t.Status.LoadBalancer.Ingress) == 0 {
// 				return false, nil
// 			}

// 			ingress := t.Status.LoadBalancer.Ingress[0]

// 			if ingress.IP == "" {
// 				return false, nil
// 			}
// 		}

// 		_ = conditions
// 		return true, nil
// 	}

// 	return false, nil
// }
