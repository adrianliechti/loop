package resource

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/adrianliechti/loop/pkg/kubernetes"

	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

	Kind string

	Name      string
	Namespace string

	Version string

	Labels      map[string]string
	Annotations map[string]string

	Status ResourceStatus
	Object interface{}
}

type ResourceStatus string

const (
	StatusPending   ResourceStatus = "Pending"
	StatusRunning   ResourceStatus = "Running"
	StatusSucceeded ResourceStatus = "Succeeded"
	StatusFailed    ResourceStatus = "Failed"
	StatusUnknown   ResourceStatus = ""
)

func App(ctx context.Context, client kubernetes.Client, namespace, name string) (*Application, error) {
	applications, err := Apps(ctx, client, namespace)

	if err != nil {
		return nil, err
	}

	for _, a := range applications {
		application := a

		if !strings.EqualFold(a.Name, name) {
			continue
		}

		return &application, nil
	}

	return nil, errors.New("application not found")
}

func Apps(ctx context.Context, client kubernetes.Client, namespace string) ([]Application, error) {
	var deployments *appsv1.DeploymentList
	var statefulsets *appsv1.StatefulSetList
	var daemonsets *appsv1.DaemonSetList
	//var replicasets *appsv1.ReplicaSetList

	var pods *corev1.PodList
	var services *corev1.ServiceList

	var ingresses *networkingv1.IngressList

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		var err error

		pods, err = client.CoreV1().Pods(namespace).
			List(ctx, metav1.ListOptions{})

		return err
	})

	eg.Go(func() error {
		var err error

		services, err = client.CoreV1().Services(namespace).
			List(ctx, metav1.ListOptions{})

		return err
	})

	eg.Go(func() error {
		var err error

		deployments, err = client.AppsV1().Deployments(namespace).
			List(ctx, metav1.ListOptions{})

		return err
	})

	eg.Go(func() error {
		var err error

		statefulsets, err = client.AppsV1().StatefulSets(namespace).
			List(ctx, metav1.ListOptions{})

		return err
	})

	eg.Go(func() error {
		var err error

		daemonsets, err = client.AppsV1().DaemonSets(namespace).
			List(ctx, metav1.ListOptions{})

		return err
	})

	// eg.Go(func() error {
	// 	var err error

	// 	replicasets, err = client.AppsV1().ReplicaSets(namespace).
	// 		List(ctx, metav1.ListOptions{})

	// 	return err
	// })

	eg.Go(func() error {
		var err error

		ingresses, err = client.NetworkingV1().Ingresses(namespace).
			List(ctx, metav1.ListOptions{})

		return err
	})

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	apps := make(map[string]*Application)

	findApp := func(object metav1.ObjectMeta, labels map[string]string) *Application {
		key, namespace, name, ok := appName(object, labels)

		if !ok {
			slog.InfoCtx(ctx, "missing app labels", "namespace", namespace, "name", name)
		}

		if app, ok := apps[key]; ok {
			return app
		}

		app := &Application{
			Name:      name,
			Namespace: namespace,

			Labels: appLabels(object, labels),
		}

		apps[key] = app
		return app
	}

	for _, daemonset := range daemonsets.Items {
		app := findApp(daemonset.ObjectMeta, daemonset.Spec.Template.Labels)

		if app == nil {
			continue
		}

		resource := Resource{
			Kind:     "DaemonSet",
			Category: "Controller",

			Name:      daemonset.Name,
			Namespace: daemonset.Namespace,

			Version: appVersion(daemonset.ObjectMeta),

			Labels:      filterLabels(daemonset.Labels),
			Annotations: filterAnnotations(daemonset.Annotations),

			Status: convertDaemonSetStatus(daemonset.Status),
			Object: daemonset,
		}

		app.Resources = append(app.Resources, resource)
	}

	for _, statefulset := range statefulsets.Items {
		app := findApp(statefulset.ObjectMeta, statefulset.Spec.Template.Labels)

		if app == nil {
			continue
		}

		resource := Resource{
			Kind:     "StatefulSet",
			Category: "Controller",

			Name:      statefulset.Name,
			Namespace: statefulset.Namespace,

			Version: appVersion(statefulset.ObjectMeta),

			Labels:      filterLabels(statefulset.Labels),
			Annotations: filterAnnotations(statefulset.Annotations),

			Status: convertStatefulSetStatus(statefulset.Status),
			Object: statefulset,
		}

		app.Resources = append(app.Resources, resource)
	}

	for _, deployment := range deployments.Items {
		app := findApp(deployment.ObjectMeta, deployment.Spec.Template.Labels)

		if app == nil {
			continue
		}

		resource := Resource{
			Kind:     "Deployment",
			Category: "Controller",

			Name:      deployment.Name,
			Namespace: deployment.Namespace,

			Version: appVersion(deployment.ObjectMeta),

			Labels:      filterLabels(deployment.Labels),
			Annotations: filterAnnotations(deployment.Annotations),

			Status: convertDeploymentStatus(deployment.Status),
			Object: deployment,
		}

		app.Resources = append(app.Resources, resource)
	}

	for _, service := range services.Items {
		if len(service.Spec.Selector) == 0 {
			continue
		}

		selector := labels.SelectorFromSet(service.Spec.Selector)

		resource := Resource{
			Kind:     "Service",
			Category: "Network",

			Name:      service.Name,
			Namespace: service.Namespace,

			Status: convertServiceStatus(service.Status),
			Object: service,
		}

		for _, app := range apps {
			if app.Namespace != service.Namespace {
				continue
			}

			var found bool

			for _, r := range app.Resources {
				if selector.Matches(labels.Set(r.Labels)) {
					found = true
				}
			}

			if !found {
				continue
			}

			app.Resources = append(app.Resources, resource)
		}
	}

	for _, ingress := range ingresses.Items {
		resource := Resource{
			Kind:     "Ingress",
			Category: "Network",

			Name:      ingress.Name,
			Namespace: ingress.Namespace,

			Status: convertIngressStatus(ingress.Status),
			Object: ingress,
		}

		var ingressServices []corev1.Service

		for _, rule := range ingress.Spec.Rules {
			if rule.IngressRuleValue.HTTP != nil {
				for _, path := range rule.IngressRuleValue.HTTP.Paths {
					if path.Backend.Service != nil {
						for _, service := range services.Items {
							if service.Namespace != ingress.Namespace {
								continue
							}

							if service.Name != path.Backend.Service.Name {
								continue
							}

							ingressServices = append(ingressServices, service)
						}
					}
				}
			}
		}

		for _, service := range ingressServices {
			app := findApp(service.ObjectMeta, service.Spec.Selector)

			if app == nil {
				continue
			}

			app.Resources = append(app.Resources, resource)
		}
	}

	for _, pod := range pods.Items {
		app := findApp(pod.ObjectMeta, pod.Labels)

		if app == nil || len(app.Resources) == 0 {
			continue
		}

		resource := Resource{
			Kind:     "Pod",
			Category: "Workload",

			Name:      pod.Name,
			Namespace: pod.Namespace,

			Status: convertPodStatus(pod.Status),
			Object: pod,
		}

		app.Resources = append(app.Resources, resource)
	}

	result := make([]Application, 0)

	for _, app := range apps {
		if len(app.Resources) > 0 {
			result = append(result, *app)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace < result[j].Namespace {
			return true
		}

		if result[i].Namespace > result[j].Namespace {
			return false
		}

		return result[i].Name < result[j].Name
	})

	return result, nil
}
