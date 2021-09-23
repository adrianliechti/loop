package kubernetes

import (
	"context"
	"io"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Client interface {
	kubernetes.Interface

	ConfigPath() string
	Config() *rest.Config

	ServicePods(ctx context.Context, namespace, name string) ([]corev1.Pod, error)
	ServicePod(ctx context.Context, namespace, name string) (*corev1.Pod, error)
	ServiceAddress(ctx context.Context, namespace, name string) (string, error)
	ServicePortForward(ctx context.Context, namespace, name, address string, ports map[int]int, readyChan chan struct{}) error

	PodExec(ctx context.Context, namespace, name, container string, command []string, tty bool, stdin io.Reader, stdout, stderr io.Writer) error
	PodPortForward(ctx context.Context, namespace, name, address string, ports map[int]int, readyChan chan struct{}) error

	WaitForPod(ctx context.Context, namespace, name string) (*corev1.Pod, error)
	WaitForService(ctx context.Context, namespace, name string) (*corev1.Service, error)
}

func New() (Client, error) {
	return NewFromConfig("")
}

func NewFromConfig(path string) (Client, error) {
	if path == "" {
		path = os.Getenv("KUBECONFIG")

		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, ".kube", "config")
		}
	}

	c, err := clientcmd.BuildConfigFromFlags("", path)

	if err != nil {
		return nil, err
	}

	cs, err := kubernetes.NewForConfig(c)

	if err != nil {
		return nil, err
	}

	client := &client{
		path:   path,
		config: c,

		Interface: cs,
	}

	return client, nil
}

type client struct {
	kubernetes.Interface

	path   string
	config *rest.Config
}

func (c *client) ConfigPath() string {
	return c.path
}

func (c *client) Config() *rest.Config {
	return c.config
}

func (c *client) ExportConfig(path string) error {
	source, err := os.Open(c.path)

	if err != nil {
		return err
	}

	defer source.Close()

	destination, err := os.Create(path)

	if err != nil {
		return err
	}

	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		return err
	}

	return nil
}
