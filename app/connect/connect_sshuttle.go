package connect

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"

	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func runShuttle(ctx context.Context, client kubernetes.Client, namespace string) error {
	kubectl := "kubectl"
	sshuttle := "sshuttle"

	name := "loop-sshuttle-" + uuid.New().String()[0:7]

	cli.Infof("Starting sshuttle pod (%s/%s)...", namespace, name)
	pod, err := createShuttle(ctx, client, namespace, name)

	if err != nil {
		return err
	}

	defer func() {
		cli.Infof("Stopping sshuttle pod (%s/%s)...", namespace, name)
		deleteShuttle(context.Background(), client, pod.Namespace, pod.Name)
	}()

	args := []string{
		"-v",
		"--method",
		"auto",
		"--dns",
		//"--to-ns=127.0.0.1",
		"-r",
		"root@localhost",
		"-e",
		kubectl + " exec -i " + pod.Name + " -n " + pod.Namespace + " -c sshuttle --kubeconfig " + client.ConfigPath() + " -- ssh",
		"10.0.0.0/8",
	}

	cmd := exec.CommandContext(ctx, sshuttle, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func shuttleLabels(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "loop-sshuttle",
		"app.kubernetes.io/instance": name,
	}
}

func createShuttle(ctx context.Context, client kubernetes.Client, namespace, name string) (*corev1.Pod, error) {
	labels := shuttleLabels(name)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},

		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "sshuttle",
					Image: "adrianliechti/loop-tunnel",
				},
				// {
				// 	Name:  "dns",
				// 	Image: "adrianliechti/loop-dns",
				// },
			},
		},
	}

	if _, err := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return nil, err
	}

	// TODO: Fix me
	for {
		time.Sleep(10 * time.Second)

		pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})

		if err != nil {
			continue
		}

		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		time.Sleep(10 * time.Second)
		return pod, nil
	}
}

func deleteShuttle(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	if err := client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		//return err
	}

	return nil
}
