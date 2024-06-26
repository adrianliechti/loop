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

	Namespace() string
	Credentials() (*Credentials, error)

	PodCIDR(ctx context.Context) (string, error)
	ServiceCIDR(ctx context.Context) (string, error)

	ServicePods(ctx context.Context, namespace, name string) ([]corev1.Pod, error)
	ServicePod(ctx context.Context, namespace, name string) (*corev1.Pod, error)
	ServiceAddress(ctx context.Context, namespace, name string) (string, error)
	ServicePortForward(ctx context.Context, namespace, name, address string, ports map[int]int, readyChan chan struct{}) error

	PodExec(ctx context.Context, namespace, name, container string, command []string, tty bool, stdin io.Reader, stdout, stderr io.Writer) error
	PodAttach(ctx context.Context, namespace, name, container string, tty bool, stdin io.Reader, stdout, stderr io.Writer) error
	PodLogs(ctx context.Context, namespace, name, container string, out io.Writer, follow bool) error
	PodPortForward(ctx context.Context, namespace, name, address string, ports map[int]int, readyChan chan struct{}) error

	WaitForPod(ctx context.Context, namespace, name string) (*corev1.Pod, error)
	WaitForService(ctx context.Context, namespace, name string) (*corev1.Service, error)

	ReadFileInPod(ctx context.Context, namespace, name, container, path string, data io.Writer) error
	CreateFileInPod(ctx context.Context, namespace, name, container, path string, data io.Reader) error
}

func New() (Client, error) {
	return NewFromConfig("")
}

func NewFromConfig(path string) (Client, error) {
	if path == "" {
		path = ConfigPath()
	}

	data, err := os.ReadFile(path)

	if err != nil {
		return nil, err
	}

	config, err := clientcmd.NewClientConfigFromBytes(data)

	if err != nil {
		return nil, err
	}

	c, err := config.ClientConfig()

	if err != nil {
		return nil, err
	}

	ns, _, _ := config.Namespace()

	if ns == "" {
		ns = "default"
	}

	cs, err := kubernetes.NewForConfig(c)

	if err != nil {
		return nil, err
	}

	client := &client{
		path:      path,
		config:    c,
		namespace: ns,

		Interface: cs,
	}

	return client, nil
}

type client struct {
	kubernetes.Interface

	path      string
	config    *rest.Config
	namespace string
}

func ConfigPath() string {
	path := os.Getenv("KUBECONFIG")

	if len(path) > 0 {
		return path
	}

	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".kube", "config")
	}

	return ""
}

func (c *client) ConfigPath() string {
	return c.path
}

func (c *client) Config() *rest.Config {
	return c.config
}

func (c *client) Namespace() string {
	return c.namespace
}
