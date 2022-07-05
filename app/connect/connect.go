package connect

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubectl"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/sshuttle"
	"github.com/adrianliechti/loop/pkg/to"

	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var Command = &cli.Command{
	Name:  "connect",
	Usage: "connect Kubernetes network",

	Flags: []cli.Flag{
		app.NamespaceFlag,
		app.KubeconfigFlag,
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)
		namespace := app.Namespace(c)

		if namespace == nil {
			namespace = to.StringPtr(client.Namespace())
		}

		return runShuttle(c.Context, client, *namespace)
	},
}

func runShuttle(ctx context.Context, client kubernetes.Client, namespace string) error {
	kubectl, _, err := kubectl.Tool(ctx)

	if err != nil {
		return err
	}

	sshuttle, _, err := sshuttle.Tool(ctx)

	if err != nil {
		return err
	}

	// Kubeadm: Services CIDR 10.96.0.0/12?, Pod CIDR 172.16.0.0/16?
	// OpenShift: Services CIDR 172.30.0.0/16, Pod CIDR 10.128.0.0/14
	cidr := "0.0.0.0/0"

	name := "loop-sshuttle-" + uuid.New().String()[0:7]

	defer func() {
		cli.Infof("Stopping sshuttle pod (%s/%s)...", namespace, name)
		deleteShuttle(context.Background(), client, namespace, name)
	}()

	cli.Infof("Starting sshuttle pod (%s/%s)...", namespace, name)
	pod, err := createShuttle(ctx, client, namespace, name)

	if err != nil {
		return err
	}

	args := []string{
		//"-v",
		"--method", "auto",
		"--dns", "--to-ns=127.0.0.1",
		"-r", "loop@localhost",
		"-e", kubectl + " exec -i " + pod.Name + " -n " + pod.Namespace + " -c sshuttle --kubeconfig " + client.ConfigPath() + " -- ssh",
		cidr,
	}

	cmd := exec.CommandContext(ctx, sshuttle, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func createShuttle(ctx context.Context, client kubernetes.Client, namespace, name string) (*corev1.Pod, error) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app.kubernetes.io/name":     "loop-sshuttle",
				"app.kubernetes.io/instance": name,
			},
		},

		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "sshuttle",
					Image: "adrianliechti/loop-tunnel:0",
				},
				{
					Name:  "dns",
					Image: "adrianliechti/loop-dns:0",
				},
			},
		},
	}

	if _, err := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return nil, err
	}

	pod, err := client.WaitForPod(ctx, namespace, name)
	time.Sleep(10 * time.Second)

	return pod, err
}

func deleteShuttle(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: to.Int64Ptr(0),
	})

	return nil
}
