package expose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/ssh"

	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var Command = &cli.Command{
	Name:  "expose",
	Usage: "expose local service",

	HideHelpCommand: true,

	Flags: []cli.Flag{
		app.NameFlag,
		app.NamespaceFlag,
		app.KubeconfigFlag,

		&cli.StringSliceFlag{
			Name:     "port",
			Usage:    "local port(s) to expose",
			Required: true,
		},
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		name := app.Name(c)
		namespace := app.Namespace(c)

		if namespace == "" {
			namespace = client.Namespace()
		}

		ports := c.StringSlice("port")

		tunnels := map[int]int{}

		for _, p := range ports {
			pair := strings.Split(p, ":")

			if len(pair) > 2 {
				return errors.New("invalid port mapping")
			}

			if len(pair) == 1 {
				pair = []string{pair[0], pair[0]}
			}

			source, err := strconv.Atoi(pair[0])

			if err != nil {
				return err
			}

			target, err := strconv.Atoi(pair[1])

			if err != nil {
				return err
			}

			tunnels[source] = target
		}

		return createTCPTunnel(c.Context, client, namespace, name, tunnels)
	},
}

func createTCPTunnel(ctx context.Context, client kubernetes.Client, namespace, name string, mapping map[int]int) error {
	options := TunnelOptions{
		ServiceType:  corev1.ServiceTypeClusterIP,
		ServicePorts: mapping,
	}

	return createTunnel(ctx, client, namespace, name, options)
}

func tunnelLabels(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "loop-tunnel",
		"app.kubernetes.io/instance": name,
	}
}

func tunnelSelector(name string) map[string]string {
	return tunnelLabels(name)
}

type TunnelOptions struct {
	ServiceType  corev1.ServiceType
	ServicePorts map[int]int
}

func createTunnel(ctx context.Context, client kubernetes.Client, namespace, name string, options TunnelOptions) error {
	if name == "" {
		name = "loop-tunnel-" + uuid.New().String()[0:7]
	}

	labels := tunnelLabels(name)
	selector := tunnelSelector(name)

	cli.Infof("Creating tunnel (%s/%s)...", namespace, name)

	defer func() {
		cli.Infof("Deleting tunnel (%s/%s)...", namespace, name)
		deleteTunnel(context.Background(), client, namespace, name)
	}()

	container := corev1.Container{
		Name:  "tunnel",
		Image: "adrianliechti/loop-tunnel",
	}

	for _, port := range options.ServicePorts {
		portSpec := corev1.ContainerPort{
			Name: fmt.Sprintf("tcp-%d", port),

			Protocol:      corev1.ProtocolTCP,
			ContainerPort: int32(port),
		}

		container.Ports = append(container.Ports, portSpec)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},

		Spec: corev1.PodSpec{
			Containers: []corev1.Container{container},
		},
	}

	if _, err := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return err
	}

	if _, err := client.WaitForPod(ctx, namespace, name); err != nil {
		return err
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},

		Spec: corev1.ServiceSpec{
			Type:     options.ServiceType,
			Selector: selector,
		},
	}

	for _, port := range options.ServicePorts {
		portSpec := corev1.ServicePort{
			Name: fmt.Sprintf("tcp-%d", port),

			Protocol: corev1.ProtocolTCP,
			Port:     int32(port),

			TargetPort: intstr.FromInt(port),
		}

		service.Spec.Ports = append(service.Spec.Ports, portSpec)
	}

	if _, err := client.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{}); err != nil {
		return err
	}

	if _, err := client.WaitForService(ctx, namespace, name); err != nil {
		return err
	}

	ready := make(chan struct{})

	go func() {
		<-ready

		cli.Info("Tunnel ready")

		for s, t := range options.ServicePorts {
			cli.Infof("Forwarding tcp://%s.%s:%d => tcp://localhost:%d", service.Name, namespace, t, s)
		}
	}()

	return connectTunnel(ctx, client, namespace, name, options.ServicePorts, ready)
}

func deleteTunnel(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	client.CoreV1().Pods(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	client.CoreV1().Services(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	client.NetworkingV1().Ingresses(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})

	return nil
}

func connectTunnel(ctx context.Context, client kubernetes.Client, namespace, name string, ports map[int]int, readyChan chan struct{}) error {
	ssh, _, err := ssh.Info(ctx)

	if err != nil {
		return err
	}

	self, err := os.Executable()

	if err != nil {
		return err
	}

	args := []string{
		"-q",
		"-l", "root",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "StrictHostKeyChecking=no",
		"-o", fmt.Sprintf("ProxyCommand=%s remote stream --kubeconfig %s --namespace %s --name %s --container ssh --port 22", self, client.ConfigPath(), namespace, name),
		"-N",
		"localhost",
	}

	for s, t := range ports {
		args = append(args, "-R", fmt.Sprintf("%d:127.0.0.1:%d", t, s))
	}

	cmd := exec.CommandContext(ctx, ssh, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	close(readyChan)

	return cmd.Run()
}
