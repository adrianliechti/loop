package catapult

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"

	"github.com/ChrisWiegman/goodhosts/v4/pkg/goodhosts"
	"github.com/hashicorp/go-multierror"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type CatapultOptions struct {
	Scope     string
	Namespace string

	Selector string
}

type tunnel struct {
	Pod corev1.Pod

	Hosts []string

	Address string
	Ports   map[string]string
}

func Start(ctx context.Context, client kubernetes.Client, options CatapultOptions) error {
	services, err := client.CoreV1().Services(options.Namespace).
		List(ctx, metav1.ListOptions{
			LabelSelector: options.Selector,
		})

	if err != nil {
		return err
	}

	var errinit error

	var tunnels []tunnel

	for _, service := range services.Items {
		if isHidden(service.Namespace) {
			continue
		}

		if len(service.Spec.Selector) == 0 {
			continue
		}

		isHeadless := strings.EqualFold(service.Spec.ClusterIP, "None")

		pods, err := client.CoreV1().Pods(service.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labels.Set(service.Spec.Selector).AsSelector().String(),
		})

		if err != nil {
			errinit = multierror.Append(errinit, err)
			continue
		}

		if !isHeadless {
			if pod, ok := primaryPod(pods.Items); ok {
				ports := portMapping(service, pod)

				if len(ports) == 0 {
					continue
				}

				address, err := addressMapping(service.Spec.ClusterIP)

				if err != nil {
					errinit = multierror.Append(errinit, err)
					continue
				}

				hosts := []string{
					fmt.Sprintf("%s.%s.svc.cluster.local", service.Name, service.Namespace),
					fmt.Sprintf("%s.%s", service.Name, service.Namespace),
				}

				if service.Namespace == options.Scope {
					hosts = append(hosts, service.Name)
				}

				tunnels = append(tunnels, tunnel{
					Pod: pod,

					Hosts: hosts,

					Address: address,
					Ports:   ports,
				})
			}
		} else {
			for _, pod := range pods.Items {
				ports := portMapping(service, pod)

				if len(ports) == 0 {
					continue
				}

				if pod.Status.PodIP == "" {
					continue
				}

				address, err := addressMapping(pod.Status.PodIP)

				if err != nil {
					errinit = multierror.Append(errinit, err)
					continue
				}

				hosts := []string{
					fmt.Sprintf("%s.%s.%s.svc.cluster.local", pod.Name, service.Name, pod.Namespace),
				}

				tunnels = append(tunnels, tunnel{
					Pod: pod,

					Hosts: hosts,

					Address: address,
					Ports:   ports,
				})
			}
		}
	}

	if errinit != nil {
		return errinit
	}

	if len(tunnels) == 0 {
		return errors.New("no services found by filter")
	}

	sort.Slice(tunnels, func(i, j int) bool {
		leftHost := tunnels[i].Hosts[0]
		leftNamesapce := tunnels[i].Pod.Namespace

		rightHost := tunnels[j].Hosts[0]
		rightNamespace := tunnels[j].Pod.Namespace

		if leftNamesapce < rightNamespace {
			return true
		}

		if leftNamesapce > rightNamespace {
			return false
		}

		if leftHost < rightHost {
			return true
		}

		if leftHost > rightHost {
			return false
		}

		return false
	})

	rows := make([][]string, 0)
	keys := []string{"Namespace", "FQDN", "Ports"}

	for _, tunnel := range tunnels {
		ports := make([]string, 0)

		for port := range tunnel.Ports {
			ports = append(ports, port)
		}

		rows = append(rows, []string{tunnel.Pod.Namespace, tunnel.Hosts[0], strings.Join(ports, ", ")})
	}

	cli.Table(keys, rows)

	var errsetup error

	hostsfile, err := goodhosts.NewHosts("Loop")

	if err != nil {
		return err
	}

	defer func() {
		hostsfile.Load()
		hostsfile.RemoveSection()
		hostsfile.Flush()
	}()

	for _, tunnel := range tunnels {
		if err := hostsfile.Add(tunnel.Address, "", tunnel.Hosts...); err != nil {
			errsetup = multierror.Append(errsetup, err)
			continue
		}

		if err := aliasIP(ctx, tunnel.Address); err != nil {
			errsetup = multierror.Append(errsetup, err)
			continue
		}

		defer unaliasIP(context.Background(), tunnel.Address)
	}

	if err := hostsfile.Flush(); err != nil {
		return err
	}

	var errtunnel error

	for _, t := range tunnels {
		tunnel := t

		go func() {
			for {
				err = forward(ctx, client, tunnel.Pod.Namespace, tunnel.Pod.Name, tunnel.Address, tunnel.Ports, nil)

				if err != nil {
					errtunnel = multierror.Append(errtunnel, err)
					return
				}

				if ctx.Err() != nil {
					break
				}

				time.Sleep(5 * time.Second)
			}
		}()
	}

	<-ctx.Done()

	return nil
}

func isHidden(namespace string) bool {
	return false
}

func primaryPod(candidates []corev1.Pod) (corev1.Pod, bool) {
	for _, pod := range candidates {
		if pod.Status.Phase == corev1.PodRunning {
			return pod, true
		}
	}

	return corev1.Pod{}, false
}

func addressMapping(ip string) (string, error) {
	parts := strings.Split(ip, ".")

	if len(parts) != 4 {
		return "", errors.New("invalid pod ip")
	}

	parts[0] = "127"
	return strings.Join(parts, "."), nil
}

func portMapping(service corev1.Service, pod corev1.Pod) map[string]string {
	ports := make(map[string]string)

	for _, port := range service.Spec.Ports {
		servicePort := int(port.Port)
		containerPort := 0

		if port.Protocol != "" && port.Protocol != corev1.ProtocolTCP {
			continue
		}

		for _, container := range pod.Spec.Containers {
			for _, p := range container.Ports {
				if p.Name != "" && p.Name == port.TargetPort.String() {
					containerPort = int(p.ContainerPort)
				}
			}
		}

		if port.TargetPort.IntVal > 0 {
			containerPort = int(port.TargetPort.IntVal)
		}

		if servicePort > 0 && containerPort > 0 {
			ports[strconv.Itoa(servicePort)] = strconv.Itoa(containerPort)
		}
	}

	return ports
}

func aliasIP(ctx context.Context, alias string) error {
	if runtime.GOOS == "darwin" {
		ifconfig := exec.CommandContext(ctx, "ifconfig", "lo0", "alias", alias)

		if err := ifconfig.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				cli.Info(string(ee.Stderr))
			}
		}
	}

	return nil
}

func unaliasIP(ctx context.Context, alias string) error {
	if runtime.GOOS == "darwin" {
		ifconfig := exec.CommandContext(ctx, "ifconfig", "lo0", "-alias", alias)

		if err := ifconfig.Run(); err != nil {
			return err
		}
	}

	return nil
}

func forward(ctx context.Context, client kubernetes.Client, namespace, name, address string, ports map[string]string, readyChan chan struct{}) error {
	if address == "" {
		address = "localhost"
	}

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, name)

	host := client.Config().Host
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")

	transport, upgrader, err := spdy.RoundTripperFor(client.Config())

	if err != nil {
		return err
	}

	mappings := make([]string, 0)

	for s, t := range ports {
		mappings = append(mappings, fmt.Sprintf("%s:%s", s, t))
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, &url.URL{Scheme: "https", Path: path, Host: host})
	forwarder, err := portforward.NewOnAddresses(dialer, []string{address}, mappings, ctx.Done(), readyChan, ioutil.Discard, ioutil.Discard)

	if err != nil {
		return err
	}

	return forwarder.ForwardPorts()
}
