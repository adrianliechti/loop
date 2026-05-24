package kubernetes

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/remotecommand"
	watchtools "k8s.io/client-go/tools/watch"
	"k8s.io/kubectl/pkg/util/term"
)

func Ptr[T any](v T) *T {
	return &v
}

func Deref[T any](ptr *T, def T) T {
	if ptr != nil {
		return *ptr
	}

	return def
}

func (c *client) ServicePods(ctx context.Context, namespace, name string) ([]corev1.Pod, error) {
	service, err := c.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	pods, err := c.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "status.phase=Running",
		LabelSelector: labels.SelectorFromSet(service.Spec.Selector).String(),
	})

	return pods.Items, err
}

func (c *client) ServicePod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	pods, err := c.ServicePods(ctx, namespace, name)

	if err != nil {
		return nil, err
	}

	if len(pods) == 0 {
		return nil, errors.New("no running pod found")
	}

	return &pods[0], nil
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
	selector := fields.OneTermEqualSelector("metadata.name", name).String()

	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = selector
			return c.CoreV1().Pods(namespace).List(ctx, options)
		},

		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = selector
			return c.CoreV1().Pods(namespace).Watch(ctx, options)
		},
	}

	ev, err := watchtools.UntilWithSync(ctx, lw, &corev1.Pod{}, nil, podReady)

	if err != nil {
		return nil, err
	}

	return ev.Object.(*corev1.Pod), nil
}

// terminalContainerWaitReasons are container Waiting reasons that won't
// resolve on their own — surfacing them as errors avoids hanging WaitForPod
// indefinitely on a pod that will never become ready.
var terminalContainerWaitReasons = map[string]struct{}{
	"ImagePullBackOff":           {},
	"ErrImagePull":               {},
	"InvalidImageName":           {},
	"CreateContainerConfigError": {},
	"CreateContainerError":       {},
	"RunContainerError":          {},
	"CrashLoopBackOff":           {},
}

func podReady(event watch.Event) (bool, error) {
	if event.Type == watch.Deleted {
		return false, errors.New("pod deleted")
	}

	p, ok := event.Object.(*corev1.Pod)

	if !ok {
		return false, nil
	}

	switch p.Status.Phase {
	case corev1.PodFailed, corev1.PodSucceeded:
		return false, errors.New("pod completed")

	case corev1.PodRunning:
		for _, cond := range p.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
	}

	for _, cs := range p.Status.ContainerStatuses {
		if cs.State.Waiting == nil {
			continue
		}

		reason := cs.State.Waiting.Reason

		if _, terminal := terminalContainerWaitReasons[reason]; !terminal {
			continue
		}

		return false, fmt.Errorf("container %s: %s: %s", cs.Name, reason, cs.State.Waiting.Message)
	}

	return false, nil
}

func (c *client) WaitForService(ctx context.Context, namespace, name string) (*corev1.Service, error) {
	selector := fields.OneTermEqualSelector("metadata.name", name).String()

	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = selector
			return c.CoreV1().Services(namespace).List(ctx, options)
		},

		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = selector
			return c.CoreV1().Services(namespace).Watch(ctx, options)
		},
	}

	ev, err := watchtools.UntilWithSync(ctx, lw, &corev1.Service{}, nil, serviceReady)

	if err != nil {
		return nil, err
	}

	return ev.Object.(*corev1.Service), nil
}

func serviceReady(event watch.Event) (bool, error) {
	if event.Type == watch.Deleted {
		return false, errors.New("service deleted")
	}

	s, ok := event.Object.(*corev1.Service)

	if !ok {
		return false, nil
	}

	if s.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return true, nil
	}

	// Managed load balancers may publish only a Hostname (e.g. AWS NLB/ELB)
	// instead of an IP, so accept either.
	for _, ing := range s.Status.LoadBalancer.Ingress {
		if ing.IP != "" || ing.Hostname != "" {
			return true, nil
		}
	}

	return false, nil
}

type terminalSizeQueueAdapter struct {
	delegate term.TerminalSizeQueue
}

func (a *terminalSizeQueueAdapter) Next() *remotecommand.TerminalSize {
	if a.delegate == nil {
		return nil
	}
	next := a.delegate.Next()
	if next == nil {
		return nil
	}
	return &remotecommand.TerminalSize{
		Width:  next.Width,
		Height: next.Height,
	}
}
