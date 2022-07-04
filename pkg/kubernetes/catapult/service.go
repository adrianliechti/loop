package catapult

import (
	"context"
	"errors"

	"github.com/adrianliechti/loop/pkg/kubernetes"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type CatapultService struct {
	client  kubernetes.Client
	options CatapultOptions

	service corev1.Service
}

func NewService(client kubernetes.Client, options CatapultOptions, service corev1.Service) (*CatapultService, error) {
	if len(service.Spec.Selector) == 0 {
		return nil, errors.New("service has no selector")
	}

	return &CatapultService{
		client:  client,
		options: options,

		service: service,
	}, nil
}

func (s *CatapultService) Tunnels(ctx context.Context) ([]*CatapultTunnel, error) {
	tunnels := make([]*CatapultTunnel, 0)

	pods, err := s.client.CoreV1().Pods(s.service.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set(s.service.Spec.Selector).AsSelector().String(),
	})

	if err != nil {
		return nil, err
	}

	if s.service.Spec.ClusterIP != corev1.ClusterIPNone {
		if pod, err := primaryPod(pods.Items); err == nil {
			t := NewTunnel(s.client, s.options, s.service, pod)
			tunnels = append(tunnels, t)
		}
	} else {
		for _, pod := range pods.Items {
			t := NewTunnel(s.client, s.options, s.service, pod)
			tunnels = append(tunnels, t)
		}
	}

	return tunnels, nil
}

func primaryPod(candidates []corev1.Pod) (corev1.Pod, error) {
	for _, pod := range candidates {
		if pod.Status.Phase == corev1.PodRunning {
			return pod, nil
		}
	}

	return corev1.Pod{}, errors.New("no running pods found")
}
