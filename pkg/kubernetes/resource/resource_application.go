package resource

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/adrianliechti/loop/pkg/kubernetes"

	"golang.org/x/sync/errgroup"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Application struct {
	Name      string
	Namespace string

	Resources []ApplicationResource
}

type ApplicationResource struct {
	Kind   string
	Object interface{}

	Name      string
	Namespace string

	Labels      map[string]interface{}
	Annotations map[string]interface{}
}

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

	findApp := func(object metav1.ObjectMeta) *Application {
		namespace, name, ok := appName(object)

		if !ok {
			return nil
		}

		key := namespace + "/" + name

		if app, ok := apps[key]; ok {
			return app
		}

		app := &Application{
			Name:      name,
			Namespace: namespace,
		}

		apps[key] = app

		return app
	}

	for _, daemonset := range daemonsets.Items {
		app := findApp(daemonset.ObjectMeta)

		if app == nil {
			continue
		}

		resource := ApplicationResource{
			Kind:   "DaemonSet",
			Object: daemonset,

			Name:      daemonset.Name,
			Namespace: daemonset.Namespace,

			Labels:      convertMap(daemonset.Labels),
			Annotations: convertMap(daemonset.Annotations),
		}

		app.Resources = append(app.Resources, resource)
	}

	for _, statefulset := range statefulsets.Items {
		app := findApp(statefulset.ObjectMeta)

		if app == nil {
			continue
		}

		resource := ApplicationResource{
			Kind:   "StatefulSet",
			Object: statefulset,

			Name:      statefulset.Name,
			Namespace: statefulset.Namespace,

			Labels:      convertMap(statefulset.Labels),
			Annotations: convertMap(statefulset.Annotations),
		}

		app.Resources = append(app.Resources, resource)
	}

	for _, deployment := range deployments.Items {
		app := findApp(deployment.ObjectMeta)

		if app == nil {
			continue
		}

		resource := ApplicationResource{
			Kind:   "Deployment",
			Object: deployment,

			Name:      deployment.Name,
			Namespace: deployment.Namespace,

			Labels:      convertMap(deployment.Labels),
			Annotations: convertMap(deployment.Annotations),
		}

		app.Resources = append(app.Resources, resource)

	}

	for _, service := range services.Items {
		app := findApp(service.ObjectMeta)

		if app == nil {
			continue
		}

		resource := ApplicationResource{
			Kind:   "Service",
			Object: service,

			Name:      service.Name,
			Namespace: service.Namespace,
		}

		app.Resources = append(app.Resources, resource)
	}

	for _, ingress := range ingresses.Items {
		app := findApp(ingress.ObjectMeta)

		if app == nil {
			continue
		}

		resource := ApplicationResource{
			Kind:   "Ingress",
			Object: ingress,

			Name:      ingress.Name,
			Namespace: ingress.Namespace,
		}

		app.Resources = append(app.Resources, resource)
	}

	for _, pod := range pods.Items {
		app := findApp(pod.ObjectMeta)

		if app == nil {
			continue
		}

		resource := ApplicationResource{
			Kind:   "Pod",
			Object: pod,

			Name:      pod.Name,
			Namespace: pod.Namespace,
		}

		app.Resources = append(app.Resources, resource)
	}

	applications := make([]Application, 0)

	for _, a := range apps {
		app := *a
		applications = append(applications, app)
	}

	sort.Slice(applications, func(i, j int) bool {
		if applications[i].Namespace < applications[j].Namespace {
			return true
		}

		if applications[i].Namespace > applications[j].Namespace {
			return false
		}

		return applications[i].Name < applications[j].Name
	})

	return applications, nil
}

func appName(object metav1.ObjectMeta) (namespace, name string, ok bool) {
	ok = false
	name = object.Name
	namespace = object.Namespace

	appInstance := object.Labels["app.kubernetes.io/instance"]

	labelApp := object.Labels["app"]
	labelRelease := object.Labels["release"]

	if labelApp != "" {
		ok = true
		name = labelApp
	}

	if labelRelease != "" {
		ok = true
		name = labelRelease
	}

	if appInstance != "" {
		ok = true
		name = appInstance
	}

	return
}

func convertMap(labels map[string]string) map[string]interface{} {
	result := map[string]interface{}{}

	for k, v := range labels {
		result[k] = v
	}

	return result
}
