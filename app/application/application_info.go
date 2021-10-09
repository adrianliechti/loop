package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/kubernetes/resource"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var infoCommand = &cli.Command{
	Name:  "info",
	Usage: "fetch application info",

	Flags: []cli.Flag{
		app.NamespaceFlag,
		&cli.StringFlag{
			Name:     "name",
			Usage:    "application name",
			Required: true,
		},
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		name := app.Name(c)
		namespace := app.Namespace(c)

		return applicationInfo(c.Context, client, namespace, name)
	},
}

func applicationInfo(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	app, err := resource.App(ctx, client, namespace, name)

	if err != nil {
		return err
	}

	for _, g := range appPods(app) {
		labelRows := make([][]string, 0)

		for k, v := range g.Labels {
			labelRows = append(labelRows, []string{k, v})
		}

		sort.Slice(labelRows, func(i, j int) bool {
			return labelRows[i][0] < labelRows[j][0]
		})

		cli.Table([]string{"LABEL", "Value"}, labelRows)

		if len(g.Pods) == 0 {
			continue
		}

		pod := g.Pods[0]
		containers := make([]corev1.Container, 0)

		containers = append(containers, pod.Spec.InitContainers...)
		containers = append(containers, pod.Spec.Containers...)

		for _, container := range containers {
			cli.Info()
			cli.Info(container.Name)
			cli.Info(strings.Repeat("=", len(container.Name)))
			cli.Info()

			rows := [][]string{
				{
					"Image", container.Image,
				},
			}

			if len(container.Command) > 0 {
				rows = append(rows, []string{
					"Command", strings.Join(container.Command, " "),
				})
			}

			if len(container.Args) > 0 {
				rows = append(rows, []string{
					"Arguments", strings.Join(container.Args, " "),
				})
			}

			cli.Info("")
			cli.Table([]string{"PROCESS", "Value"}, rows)

			if len(container.Env) > 0 {
				rows := make([][]string, 0)

				for _, e := range container.Env {
					key := e.Name

					value, err := envVariable(ctx, client, pod.Namespace, e)

					if err != nil {
						continue
					}

					rows = append(rows, []string{key, value})
				}

				cli.Info("")
				cli.Table([]string{"Environment", "Value"}, rows)
			}

			if len(container.Ports) > 0 {
				rows := make([][]string, 0)

				for _, port := range container.Ports {
					rows = append(rows, []string{port.Name, fmt.Sprintf("%d", port.ContainerPort)})
				}

				cli.Info("")
				cli.Table([]string{"Port", "Value"}, rows)
			}

			if len(container.VolumeMounts) > 0 {
				rows := make([][]string, 0)

				for _, volume := range container.VolumeMounts {
					rows = append(rows, []string{volume.Name, volume.MountPath})
				}

				cli.Info("")
				cli.Table([]string{"Volume", "Value"}, rows)
			}
		}
	}

	return nil
}

type podGroup struct {
	Labels labels.Set

	Pods []corev1.Pod
}

func appPods(app *resource.Application) []podGroup {
	result := make([]podGroup, 0)

	pods := make([]corev1.Pod, 0)

	for _, r := range app.Resources {
		if pod, ok := r.Object.(corev1.Pod); ok {
			pods = append(pods, pod)
		}
	}

	for _, i := range pods {
		items := make([]corev1.Pod, 0)
		selector := labels.SelectorFromSet(i.Labels)

		for _, pod := range pods {
			podLabels := labels.Set(pod.Labels)

			if !selector.Matches(podLabels) {
				continue
			}

			items = append(items, pod)
		}

		result = append(result, podGroup{
			Labels: labels.Set(i.Labels),
			Pods:   items,
		})
	}

	return result
}

func envVariable(ctx context.Context, client kubernetes.Client, namespace string, e corev1.EnvVar) (string, error) {
	if e.Value != "" {
		return e.Value, nil
	}

	if e.ValueFrom != nil {
		if e.ValueFrom.SecretKeyRef != nil {
			secretName := e.ValueFrom.SecretKeyRef.Name
			secretKey := e.ValueFrom.SecretKeyRef.Key

			secret, err := client.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})

			if err != nil {
				return "", err
			}

			value := secret.Data[secretKey]

			if value == nil {
				return "", errors.New("secret key not found")
			}

			return string(value), nil
		}

		if e.ValueFrom.ConfigMapKeyRef != nil {
			configName := e.ValueFrom.ConfigMapKeyRef.Name
			configKey := e.ValueFrom.ConfigMapKeyRef.Key

			config, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, configName, metav1.GetOptions{})

			if err != nil {
				return "", err
			}

			value := config.Data[configKey]

			if value == "" {
				return "", errors.New("config key not found")
			}

			return string(value), nil
		}
	}

	return "", errors.New("unable to get value")
}
