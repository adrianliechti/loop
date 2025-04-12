package expose

import (
	"context"
	"fmt"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/ssh"
	"github.com/adrianliechti/loop/pkg/system"

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

		app.PortsFlag,
	},

	Action: func(ctx context.Context, cmd *cli.Command) error {
		client := app.MustClient(ctx, cmd)

		name := app.Name(ctx, cmd)
		namespace := app.Namespace(ctx, cmd)

		if namespace == "" {
			namespace = client.Namespace()
		}

		tunnels := app.MustPorts(ctx, cmd)

		return CreateTunnel(ctx, client, namespace, name, tunnels)
	},
}

func CreateTunnel(ctx context.Context, client kubernetes.Client, namespace, name string, mapping map[int]int) error {
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

	cli.Infof("★ creating tunnel (%s/%s)...", namespace, name)

	defer func() {
		cli.Infof("★ removing tunnel (%s/%s)...", namespace, name)
		deleteTunnel(context.Background(), client, namespace, name)
	}()

	container := corev1.Container{
		Name:  "tunnel",
		Image: "ghcr.io/adrianliechti/loop-tunnel",
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

		for s, t := range options.ServicePorts {
			cli.Infof("★ forwarding tcp://%s.%s:%d => tcp://localhost:%d", service.Name, namespace, t, s)
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
	localport, err := system.FreePort(0)

	if err != nil {
		return err
	}

	options := []ssh.Option{}

	for s, t := range ports {
		options = append(options, ssh.WithRemotePortForward(ssh.PortForward{LocalPort: t, RemotePort: s}))
	}

	ssh := ssh.New(fmt.Sprintf("127.0.0.1:%d", localport), options...)

	ready := make(chan struct{})

	go func() {
		<-ready

		close(readyChan)

		if err := ssh.Run(ctx); err != nil {
			cli.Error(err)
		}
	}()

	if err := client.PodPortForward(ctx, namespace, name, "", map[int]int{localport: 22}, ready); err != nil {
		return err
	}

	return nil
}
