package docker

import (
	"context"
	"errors"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Delete(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	if namespace == "" {
		namespace = client.Namespace()
	}

	cli.Infof("★ Deleting daemon (%s/%s)...", namespace, name)

	labelSelector := metav1.FormatLabelSelector(&metav1.LabelSelector{
		MatchLabels: resourceLabels(name),
	})

	listOptions := metav1.ListOptions{
		LabelSelector: labelSelector,
	}

	deleteOptions := metav1.DeleteOptions{}

	var result error

	// Pods are owned by the StatefulSet and deleted via cascade. PVCs are
	// created from VolumeClaimTemplates but are *not* garbage-collected when
	// the StatefulSet is deleted, so they need an explicit cleanup pass.
	if err := client.AppsV1().StatefulSets(namespace).DeleteCollection(ctx, deleteOptions, listOptions); err != nil {
		result = errors.Join(result, err)
	}

	if err := client.CoreV1().PersistentVolumeClaims(namespace).DeleteCollection(ctx, deleteOptions, listOptions); err != nil {
		result = errors.Join(result, err)
	}

	return result
}
