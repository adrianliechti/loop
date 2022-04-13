package expose

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubectl"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/ssh"
	"github.com/adrianliechti/loop/pkg/to"

	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var Command = &cli.Command{
	Name:  "expose",
	Usage: "expose local servers",

	Category: app.CategoryDevelopment,

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		tcpCommand,
		httpCommand,
	},
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
	ServiceHost  string
	ServicePorts map[int]int

	IngressHost    string
	IngressMapping map[string]int
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

	if options.ServiceHost != "" {
		service.Annotations = map[string]string{
			"external-dns.alpha.kubernetes.io/ttl":      "10",
			"external-dns.alpha.kubernetes.io/hostname": strings.TrimRight(options.ServiceHost, ".") + ".",
		}
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

	if options.IngressHost != "" && len(options.IngressMapping) > 0 {
		paths := make([]networkingv1.HTTPIngressPath, 0)

		for k, v := range options.IngressMapping {
			path := k
			pathType := networkingv1.PathTypePrefix

			port := int32(v)

			pathSpec := networkingv1.HTTPIngressPath{
				Path:     path,
				PathType: &pathType,

				Backend: networkingv1.IngressBackend{
					Service: &networkingv1.IngressServiceBackend{
						Name: name,
						Port: networkingv1.ServiceBackendPort{
							Number: *to.Int32Ptr(port),
						},
					},
				},
			}

			paths = append(paths, pathSpec)
		}

		ingress := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: labels,

				Annotations: map[string]string{
					"kubernetes.io/tls-acme": "true",
				},
			},

			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{
					{
						Host: options.IngressHost,
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: paths,
							},
						},
					},
				},

				TLS: []networkingv1.IngressTLS{
					{
						SecretName: name + "-tls",
						Hosts: []string{
							options.IngressHost,
						},
					},
				},
			},
		}

		if _, err := client.NetworkingV1().Ingresses(namespace).Create(ctx, ingress, metav1.CreateOptions{}); err != nil {
			return err
		}
	}

	ready := make(chan struct{})

	go func() {
		<-ready

		cli.Info("Tunnel ready")

		if options.IngressHost != "" {
			for path := range options.IngressMapping {
				cli.Infof("Forwarding http://%s%s", options.IngressHost, path)
			}
		}

		if options.ServiceHost != "" {
			for s, t := range options.ServicePorts {
				cli.Infof("Forwarding tcp://%s:%d => tcp://localhost:%d", options.ServiceHost, t, s)
			}
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
	ssh, _, err := ssh.Tool(ctx)

	if err != nil {
		return err
	}

	kubectl, _, err := kubectl.Tool(ctx)

	if err != nil {
		return err
	}

	args := []string{
		"-q",
		"-l", "root",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "StrictHostKeyChecking=no",
		"-o", "ProxyCommand=" + kubectl + " exec -i -n " + namespace + " " + name + " --kubeconfig " + client.ConfigPath() + "  -- nc 127.0.0.1 22",
		"localhost",
		"-N",
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
