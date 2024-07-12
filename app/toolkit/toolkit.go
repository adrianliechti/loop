package toolkit

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

var Command = &cli.Command{
	Name:  "toolkit",
	Usage: "run cluster tool",

	Flags: []cli.Flag{
		app.NamespaceFlag,
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		namespace := app.Namespace(c)

		if namespace == "" {
			namespace = client.Namespace()
		}

		command := c.Args().Slice()
		return RunToolKit(c.Context, client, namespace, command)
	},
}

func RunToolKit(ctx context.Context, client kubernetes.Client, namespace string, command []string) error {
	if namespace == "" {
		namespace = client.Namespace()
	}

	name := "loop-toolkit-" + uuid.New().String()[0:7]
	container := "toolkit"

	creds, err := client.Credentials()

	if err != nil {
		return err
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
					Name: container,

					Image:           "ghcr.io/adrianliechti/loop-toolkit",
					ImagePullPolicy: corev1.PullAlways,

					TTY:   true,
					Stdin: true,

					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "config",
							MountPath: "/var/run/secrets/kubernetes.io/serviceaccount",
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

	if len(command) > 0 {
		return client.PodExec(ctx, namespace, name, container, command, true, os.Stdin, os.Stdout, os.Stderr)
	}

	return client.PodAttach(ctx, namespace, name, container, true, os.Stdin, os.Stdout, os.Stderr)
}
