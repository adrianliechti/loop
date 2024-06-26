package remote

import (
	"context"
	"os"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/to"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var kubectlCommand = &cli.Command{
	Name:  "kubectl",
	Usage: "run cluster kubectl",

	Flags: []cli.Flag{
		app.NamespaceFlag,
		app.KubeconfigFlag,
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		namespace := app.Namespace(c)

		return runKubectl(c.Context, client, namespace)
	},
}

func runKubectl(ctx context.Context, client kubernetes.Client, namespace string) error {
	if namespace == "" {
		namespace = client.Namespace()
	}

	name := "loop-kubectl-" + uuid.New().String()[0:7]

	creds, err := client.Credentials()

	if err != nil {
		return err
	}

	args := []string{
		"get",
		"nodes",
	}

	if _, err := client.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},

		Data: map[string][]byte{
			"ca.crt": creds.CAData,
			"token":  []byte(creds.Token),
		},
	}, metav1.CreateOptions{}); err != nil {
		return err
	}

	defer func() {
		client.CoreV1().Secrets(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	}()

	if _, err := client.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},

		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "kubectl",

					Image:           "bitnami/kubectl",
					ImagePullPolicy: corev1.PullAlways,

					Env: []corev1.EnvVar{
						{
							Name:  "KUBERNETES_SERVICE_HOST",
							Value: creds.Host,
						},
						{
							Name:  "KUBERNETES_SERVICE_PORT",
							Value: creds.Port,
						},
					},

					Args: args,

					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "config",
							MountPath: "/var/run/secrets/kubernetes.io/serviceaccount/token",
							SubPath:   "token",
							ReadOnly:  true,
						},
						{
							Name:      "config",
							MountPath: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
							SubPath:   "ca.crt",
							ReadOnly:  true,
						},
					},
				},
			},

			RestartPolicy: corev1.RestartPolicyNever,

			TerminationGracePeriodSeconds: to.Ptr(int64(10)),

			Volumes: []corev1.Volume{
				{
					Name: "config",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: name,
						},
					},
				},
			},
		},
	}, metav1.CreateOptions{}); err != nil {
		return err
	}

	defer func() {
		client.CoreV1().Pods(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	}()

	if _, err := client.WaitForPod(ctx, namespace, name); err != nil {
		return err
	}

	return client.PodAttach(ctx, namespace, name, "kubectl", true, nil, os.Stdout, os.Stderr)
}
