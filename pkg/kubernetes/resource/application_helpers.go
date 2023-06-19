package resource

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type Application struct {
	Name      string
	Namespace string

	Version string

	Labels    map[string]string
	Resources []Resource
}

type Resource struct {
	Category string

	Kind   string
	Object interface{}

	Name      string
	Namespace string

	Version string

	Labels      map[string]string
	Annotations map[string]string

	Status ResourceStatus
}

type ResourceStatus string

const (
	StatusPending   ResourceStatus = "Pending"
	StatusRunning   ResourceStatus = "Running"
	StatusSucceeded ResourceStatus = "Succeeded"
	StatusFailed    ResourceStatus = "Failed"
	StatusUnknown   ResourceStatus = ""
)

func appName(object metav1.ObjectMeta, labels map[string]string) (key, namespace, name string, ok bool) {
	name = object.Name
	namespace = object.Namespace

	if labels == nil {
		labels = object.Labels
	}

	var app string
	var instance string
	var component string

	// name of the application (e.g. mysql)
	for _, key := range []string{"k8s-app", "app", "app.kubernetes.io/name"} {
		if value, ok := labels[key]; ok {
			app = value
		}
	}

	// unique name identifying the instance of an application (e.g. mysql-abcxzy)
	for _, key := range []string{"release", "app.kubernetes.io/instance"} {
		if value, ok := labels[key]; ok {
			instance = value
		}
	}

	// component within the architecture
	for _, key := range []string{"component", "app.kubernetes.io/component"} {
		if value, ok := labels[key]; ok {
			component = value
		}
	}

	if instance != "" && app != "" {
		name = instance

		if app != instance {
			name = fmt.Sprintf("%s (%s)", instance, app)
		}
	} else if instance != "" {
		name = instance
	} else if app != "" {
		name = app
	}

	ok = instance != "" || app != "" || component != ""
	key = strings.Join([]string{namespace, instance, app, component}, "/")

	return
}

func appVersion(object metav1.ObjectMeta) string {
	var version string

	for _, k := range []string{"app.kubernetes.io/version"} {
		if v, ok := object.Labels[k]; ok {
			version = v
		}
	}

	return version
}

func appLabels(object metav1.ObjectMeta, labels map[string]string) map[string]string {
	// https://github.com/grafana/loki/blob/master/production/ksonnet/promtail/scrape_config.libsonnet

	result := map[string]string{
		"namespace": object.Namespace,
	}

	if labels == nil {
		labels = object.Labels
	}

	for _, k := range []string{object.Name, "app", "app.kubernetes.io/name"} {
		if v, ok := labels[k]; ok {
			result["app"] = v
		}
	}

	for _, k := range []string{"release", "app.kubernetes.io/instance"} {
		if v, ok := labels[k]; ok {
			result["instance"] = v
		}
	}

	for _, k := range []string{"component", "app.kubernetes.io/component"} {
		if v, ok := labels[k]; ok {
			result["component"] = v
		}
	}

	return result
}

func filterLabels(labels map[string]string) map[string]string {
	return labels
	// result := map[string]string{}

	// for k, v := range labels {
	// 	result[k] = v
	// }

	// return result
}

func filterAnnotations(annotations map[string]string) map[string]string {
	result := map[string]string{}

	for k, v := range annotations {
		if strings.EqualFold(k, "kubectl.kubernetes.io/last-applied-configuration") {
			continue
		}

		result[k] = v
	}

	return result
}

func convertDaemonSetStatus(status appsv1.DaemonSetStatus) ResourceStatus {
	if status.DesiredNumberScheduled == 0 {
		// TODO: Do we need a Stopped State (e.g. on Nodes, or Controllers with 0 replicas)
		return StatusRunning
	}

	if status.NumberAvailable >= status.DesiredNumberScheduled {
		return StatusRunning
	}

	return StatusPending
}

func convertStatefulSetStatus(status appsv1.StatefulSetStatus) ResourceStatus {
	if status.Replicas == 0 {
		// TODO: Do we need a Stopped State (e.g. on Nodes, or Controllers with 0 replicas)
		return StatusRunning
	}

	if status.AvailableReplicas >= status.Replicas {
		return StatusRunning
	}

	return StatusPending
}

func convertDeploymentStatus(status appsv1.DeploymentStatus) ResourceStatus {
	if status.Replicas == 0 {
		// TODO: Do we need a Stopped State (e.g. on Nodes, or Controllers with 0 replicas)
		return StatusRunning
	}

	if status.AvailableReplicas >= status.Replicas {
		return StatusRunning
	}

	return StatusPending
}

func convertServiceStatus(status corev1.ServiceStatus) ResourceStatus {
	for _, ingress := range status.LoadBalancer.Ingress {
		if ingress.IP == "" && ingress.Hostname == "" {
			return StatusPending
		}
	}

	return StatusRunning
}

func convertIngressStatus(status networkingv1.IngressStatus) ResourceStatus {
	for _, ingress := range status.LoadBalancer.Ingress {
		if ingress.IP == "" && ingress.Hostname == "" {
			return StatusPending
		}
	}

	return StatusRunning
}

func convertPodStatus(status corev1.PodStatus) ResourceStatus {
	switch status.Phase {
	case corev1.PodPending:
		return StatusPending

	case corev1.PodRunning:
		return StatusRunning

	case corev1.PodSucceeded:
		return StatusSucceeded

	case corev1.PodFailed:
		return StatusFailed

	default:
		return StatusUnknown
	}
}

func findReplicaSets(items []appsv1.ReplicaSet, reference types.UID) []appsv1.ReplicaSet {
	result := make([]appsv1.ReplicaSet, 0)

	for _, replica := range items {
		var found bool

		for _, owner := range replica.OwnerReferences {
			if strings.EqualFold(string(owner.UID), string(reference)) {
				found = true
				break
			}
		}

		if found {
			result = append(result, replica)
		}
	}

	return result
}
