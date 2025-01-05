package kubernetes

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/Masterminds/semver/v3"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	gateway "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
	gatewayv1 "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/typed/apis/v1"

	apiextensions "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
)

type Client interface {
	kubernetes.Interface
	dynamic.Interface

	GatewayV1() gatewayv1.GatewayV1Interface
	ApiextensionsV1() apiextensionsv1.ApiextensionsV1Interface

	Config() *rest.Config

	Namespace() string
	Credentials() (*Credentials, error)

	Version(ctx context.Context) (*semver.Version, error)

	Apply(ctx context.Context, namespace string, reader io.Reader) error
	ApplyFile(ctx context.Context, namespace string, path string) error
	ApplyURL(ctx context.Context, namespace string, url string) error

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
	return NewFromFile("")
}

func NewFromFile(kubeconfig string) (Client, error) {
	if kubeconfig == "" {
		kubeconfig = ConfigPath()
	}

	data, err := os.ReadFile(kubeconfig)

	if err != nil {
		return nil, err
	}

	return NewFromBytes(data)
}

func NewFromBytes(kubeconfig []byte) (Client, error) {
	config, err := clientcmd.NewClientConfigFromBytes(kubeconfig)

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

	return NewFromConfig(c, ns)
}

func NewFromConfig(config *rest.Config, namespace string) (Client, error) {
	c, err := kubernetes.NewForConfig(config)

	if err != nil {
		return nil, err
	}

	dc, err := dynamic.NewForConfig(config)

	if err != nil {
		return nil, err
	}

	gc, err := gateway.NewForConfig(config)

	if err != nil {
		return nil, err
	}

	ec, err := apiextensions.NewForConfig(config)

	if err != nil {
		return nil, err
	}

	client := &client{
		config:    config,
		namespace: namespace,

		Interface: c,
		dynamic:   dc,

		gateway:       gc,
		apiextensions: ec,
	}

	return client, nil
}

type client struct {
	config    *rest.Config
	namespace string

	kubernetes.Interface
	dynamic dynamic.Interface

	gateway       gateway.Interface
	apiextensions apiextensions.Interface
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

func (c *client) Config() *rest.Config {
	return c.config
}

func (c *client) Namespace() string {
	return c.namespace
}

func (c *client) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return c.dynamic.Resource(resource)
}

func (c client) GatewayV1() gatewayv1.GatewayV1Interface {
	return c.gateway.GatewayV1()
}

func (c client) ApiextensionsV1() apiextensionsv1.ApiextensionsV1Interface {
	return c.apiextensions.ApiextensionsV1()
}
