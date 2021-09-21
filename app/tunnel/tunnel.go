package tunnel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/to"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var Command = &cli.Command{
	Name:  "tunnel",
	Usage: "expose local servers",

	HideHelpCommand: true,

	Subcommands: []*cli.Command{
		{
			Name:  "list",
			Usage: "list tunnels",

			Flags: []cli.Flag{
				app.NamespaceFlag,
			},

			Action: func(c *cli.Context) error {
				client := app.MustClient(c)
				namespace := app.Namespace(c)

				return listTunnels(c.Context, client, namespace)
			},
		},
		{
			Name:  "create",
			Usage: "create a tunnel",

			Flags: []cli.Flag{
				app.NameFlag,
				app.NamespaceFlag,
				&cli.StringFlag{
					Name:        app.PortFlag.Name,
					Usage:       "local server port",
					DefaultText: "8080",
				},
				&cli.StringFlag{
					Name:  "host",
					Usage: "hostname",
				},
			},

			Action: func(c *cli.Context) error {
				client := app.MustClient(c)

				name := app.MustName(c)
				namespace := app.NamespaceOrDefault(c)

				host := c.String("host")

				if host == "" {
					cli.Fatal("host missing")
				}

				return createHTTPTunnel(c.Context, client, namespace, name, host)
			},
		},
		{
			Name:  "delete",
			Usage: "delete a tunnel",

			Flags: []cli.Flag{
				app.NameFlag,
				app.NamespaceFlag,
			},

			Action: func(c *cli.Context) error {
				client := app.MustClient(c)

				name := app.MustName(c)
				namespace := app.NamespaceOrDefault(c)

				return deleteTunnel(c.Context, client, namespace, name)
			},
		},
		{
			Name:  "connect",
			Usage: "conenct to a tunnel",

			Flags: []cli.Flag{
				app.NameFlag,
				app.NamespaceFlag,
				app.PortFlag,
			},

			Action: func(c *cli.Context) error {
				client := app.MustClient(c)

				name := app.MustName(c)
				namespace := app.NamespaceOrDefault(c)

				port := app.MustPort(c)

				return connectTunnel(c.Context, client, namespace, name, port)
			},
		},
	},
}

func tunnelsSelector() string {
	return "app.kubernetes.io/name=loop-tunnel"
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

func listTunnels(ctx context.Context, client kubernetes.Client, namespace string) error {
	services, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: tunnelsSelector(),
	})

	if err != nil {
		return err
	}

	for _, i := range services.Items {
		service := i

		if service.DeletionTimestamp != nil {
			continue
		}

		fmt.Printf("%s/%s\n", service.Namespace, service.Name)
	}

	return nil
}

func createHTTPTunnel(ctx context.Context, client kubernetes.Client, namespace, name, host string) error {
	path := "/"
	pathType := networkingv1.PathTypePrefix

	labels := tunnelLabels(name)
	selector := tunnelSelector(name)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},

		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,

			Selector: selector,

			Ports: []corev1.ServicePort{
				{
					Name: "http",

					Protocol: corev1.ProtocolTCP,
					Port:     int32(80),

					TargetPort: intstr.FromString("http"),
				},
			},
		},
	}

	if _, err := client.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{}); err != nil {
		return err
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
					Host: host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     path,
									PathType: &pathType,

									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: name,
											Port: networkingv1.ServiceBackendPort{
												Number: *to.Int32Ptr(80),
											},
										},
									},
								},
							},
						},
					},
				},
			},

			TLS: []networkingv1.IngressTLS{
				{
					SecretName: name + "-tls",
					Hosts: []string{
						host,
					},
				},
			},
		},
	}

	if _, err := client.NetworkingV1().Ingresses(namespace).Create(ctx, ingress, metav1.CreateOptions{}); err != nil {
		return err
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},

		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "tunnel",
					Image: "adrianliechti/loop-tunnel",

					Ports: []corev1.ContainerPort{
						{
							Name:          "http",
							Protocol:      corev1.ProtocolTCP,
							ContainerPort: 8080,
						},
					},
				},
			},
		},
	}

	if _, err := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return err
	}

	return nil
}

func deleteTunnel(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	if err := client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		//return err
	}

	if err := client.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		//return err
	}

	if err := client.NetworkingV1().Ingresses(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		//return err
	}

	// if err := client.CoreV1().Secrets(namespace).Delete(ctx, name+"-tls", metav1.DeleteOptions{}); err != nil {
	// 	//return err
	// }

	return nil
}

func connectTunnel(ctx context.Context, client kubernetes.Client, namespace, name, port string) error {
	ingress, err := client.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})

	if err != nil {
		return err
	}

	if len(ingress.Spec.Rules) == 0 || ingress.Spec.Rules[0].Host == "" {
		return errors.New("invalid tunnel")
	}

	host := ingress.Spec.Rules[0].Host

	ssh := "ssh"
	kubectl := "kubectl"

	args := []string{
		"-q",
		"-l",
		"root",
		"-o",
		"UserKnownHostsFile=/dev/null",
		"-o",
		"StrictHostKeyChecking=no",
		"-o",
		"ProxyCommand=" + kubectl + " exec -i -n " + namespace + " " + name + " --kubeconfig " + client.ConfigPath() + "  -- nc 127.0.0.1 22",
		"localhost",
		"-N",
		"-R",
		"8080:127.0.0.1:" + port,
	}

	cmd := exec.CommandContext(ctx, ssh, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cli.Infof("Forwarding https://%s => http://localhost:%s", host, port)

	return cmd.Run()
}
