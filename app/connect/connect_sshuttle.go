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
	rbacv1 "k8s.io/api/rbac/v1"
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

	account := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}

	if _, err := client.CoreV1().ServiceAccounts(namespace).Create(ctx, account, metav1.CreateOptions{}); err != nil {
		return nil, err
	}

	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},

		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: "cluster-admin",
		},

		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      account.Name,
				Namespace: namespace,
			},
		},
	}

	if _, err := client.RbacV1().ClusterRoleBindings().Create(ctx, binding, metav1.CreateOptions{}); err != nil {
		return nil, err
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},

		Spec: corev1.PodSpec{
			ServiceAccountName: account.Name,

			Containers: []corev1.Container{
				{
					Name:  "sshuttle",
					Image: "adrianliechti/loop-tunnel",
				},
				{
					Name:  "dns",
					Image: "adrianliechti/loop-dns",
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
	if err := client.RbacV1().ClusterRoleBindings().Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		//return err
	}

	if err := client.CoreV1().ServiceAccounts(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		//return err
	}

	if err := client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		//return err
	}

	return nil
}
