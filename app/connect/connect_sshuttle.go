package connect

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubectl"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/sshuttle"

	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func runShuttle(ctx context.Context, client kubernetes.Client, namespace string) error {
	kubectl, _, err := kubectl.Tool(ctx)

	if err != nil {
		return err
	}

	sshuttle, _, err := sshuttle.Tool(ctx)

	if err != nil {
		return err
	}

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
		"-v",
		"--method",
		"auto",
		"--dns",
		"--to-ns=127.0.0.1",
		"-r",
		"loop@localhost",
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
					ImagePullPolicy: corev1.PullAlways,

					Name:  "sshuttle",
					Image: "adrianliechti/loop-tunnel:new",

					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
				},
				{
					ImagePullPolicy: corev1.PullAlways,

					Name:  "dns",
					Image: "adrianliechti/loop-dns:new",

					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
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
	if err := client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		//return err
	}

	return nil
}
