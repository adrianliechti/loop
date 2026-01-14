package docker

import (
	"context"

	"github.com/adrianliechti/loop/pkg/kubernetes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Daemon struct {
	Name      string
	Namespace string
}

type ListOptions struct {
	Namespace string
}

func List(ctx context.Context, client kubernetes.Client, options *ListOptions) ([]Daemon, error) {
	if options == nil {
		options = new(ListOptions)
	}

	pods, err := client.CoreV1().Pods(options.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: resourceLabelSelector,
	})

	if err != nil {
		return nil, err
	}

	var result []Daemon

	for _, p := range pods.Items {
		name := p.Labels["loop.cluster.local/docker"]

		if name == "" {
			continue
		}

		daemon := Daemon{
			Name:      name,
			Namespace: p.Namespace,
		}

		result = append(result, daemon)
	}

	return result, nil
}
