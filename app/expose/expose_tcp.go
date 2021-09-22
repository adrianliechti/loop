package expose

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubectl"
	"github.com/adrianliechti/loop/pkg/kubernetes"

	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var tcpCommand = &cli.Command{
	Name:  "tcp",
	Usage: "expose tcp server",

	Flags: []cli.Flag{
		app.NameFlag,
		app.NamespaceFlag,
		&cli.IntSliceFlag{
			Name:     app.PortsFlag.Name,
			Usage:    "local port(s) to expose",
			Required: true,
		},
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		name := app.Name(c)
		namespace := app.NamespaceOrDefault(c)

		ports := app.MustPorts(c)

		mapping := map[int]int{}

		for _, p := range ports {
			mapping[p] = p
		}

		options := tunnelOptions{
			serviceType:  corev1.ServiceTypeLoadBalancer,
			servicePorts: mapping,
		}

		return createTCPTunnel(c.Context, client, namespace, name, options)
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

type tunnelOptions struct {
	serviceType  corev1.ServiceType
	servicePorts map[int]int
}

func createTCPTunnel(ctx context.Context, client kubernetes.Client, namespace, name string, options tunnelOptions) error {
	if name == "" {
		name = "loop-tunnel-" + uuid.New().String()[0:7]
	}

	if options.serviceType == "" {
		options.serviceType = corev1.ServiceTypeClusterIP
	}

	labels := tunnelLabels(name)
	selector := tunnelSelector(name)

	container := corev1.Container{
		Name:  "tunnel",
		Image: "adrianliechti/loop-tunnel",
	}

	for _, port := range options.servicePorts {
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

	cli.Infof("Waiting for tunnel pod (%s/%s)...", namespace, name)
	if _, err := client.WaitForPod(ctx, namespace, name); err != nil {
		return err
	}

	defer func() {
		cli.Infof("Deleting tunnel pod (%s/%s)...", namespace, name)
		client.CoreV1().Pods(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	}()

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},

		Spec: corev1.ServiceSpec{
			Type:     options.serviceType,
			Selector: selector,
		},
	}

	for _, port := range options.servicePorts {
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

	defer func() {
		cli.Infof("Deleting tunnel service (%s/%s)...", namespace, name)
		client.CoreV1().Services(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	}()

	cli.Infof("Waiting for tunnel service (%s/%s)...", namespace, name)
	if _, err := client.WaitForService(ctx, namespace, name); err != nil {
		return err
	}

	ready := make(chan struct{})

	go func() {
		<-ready

		address, _ := client.ServiceAddress(ctx, namespace, name)

		for s, t := range options.servicePorts {
			cli.Infof("Forwarding tcp://%s:%d => http://localhost:%d", address, t, s)
		}
	}()

	return tunnelTCP(ctx, client, namespace, name, options.servicePorts, ready)
}

func tunnelTCP(ctx context.Context, client kubernetes.Client, namespace, name string, ports map[int]int, readyChan chan struct{}) error {
	ssh := "ssh"

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

// 	ingress := &networkingv1.Ingress{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:   name,
// 			Labels: labels,

// 			Annotations: map[string]string{
// 				"kubernetes.io/tls-acme": "true",
// 			},
// 		},

// 		Spec: networkingv1.IngressSpec{
// 			Rules: []networkingv1.IngressRule{
// 				{
// 					Host: host,
// 					IngressRuleValue: networkingv1.IngressRuleValue{
// 						HTTP: &networkingv1.HTTPIngressRuleValue{
// 							Paths: []networkingv1.HTTPIngressPath{
// 								{
// 									Path:     path,
// 									PathType: &pathType,

// 									Backend: networkingv1.IngressBackend{
// 										Service: &networkingv1.IngressServiceBackend{
// 											Name: name,
// 											Port: networkingv1.ServiceBackendPort{
// 												Number: *to.Int32Ptr(80),
// 											},
// 										},
// 									},
// 								},
// 							},
// 						},
// 					},
// 				},
// 			},

// 			TLS: []networkingv1.IngressTLS{
// 				{
// 					SecretName: name + "-tls",
// 					Hosts: []string{
// 						host,
// 					},
// 				},
// 			},
// 		},
// 	}

// 	if _, err := client.NetworkingV1().Ingresses(namespace).Create(ctx, ingress, metav1.CreateOptions{}); err != nil {
// 		return err
// 	}
