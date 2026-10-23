package tunnel

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/ssh"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

type podTestClient struct {
	kubernetes.Client
	core typedcorev1.CoreV1Interface
	wait func(context.Context, string, string) (*corev1.Pod, error)
}

func (c *podTestClient) Namespace() string {
	return "default"
}

func (c *podTestClient) CoreV1() typedcorev1.CoreV1Interface {
	return c.core
}

func (c *podTestClient) WaitForPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	return c.wait(ctx, namespace, name)
}

func TestRunOnCreateCustomizesPod(t *testing.T) {
	ctx := context.Background()
	clientset := fake.NewClientset()
	client := &podTestClient{core: clientset.CoreV1()}
	waitErr := errors.New("stop after pod creation")
	pullSecrets := []corev1.LocalObjectReference{{Name: "registry-credentials"}}
	hookCalled := false
	waitCalled := false

	client.wait = func(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
		waitCalled = true

		if namespace != "custom" || name != "custom-tunnel" {
			t.Fatalf("waiting for %s/%s, want custom/custom-tunnel", namespace, name)
		}

		pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})

		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(pod.Spec.ImagePullSecrets, pullSecrets) {
			t.Errorf("image pull secrets = %v, want %v", pod.Spec.ImagePullSecrets, pullSecrets)
		}

		if pod.Spec.Containers[0].Image != "registry.example.com/tunnel:custom" {
			t.Errorf("image override was not applied: %s", pod.Spec.Containers[0].Image)
		}

		if pod.Spec.ServiceAccountName != "tunnel-account" {
			t.Errorf("service account override was not applied: %s", pod.Spec.ServiceAccountName)
		}

		return nil, waitErr
	}

	err := Run(ctx, client, &RunOptions{
		Forwards: []ssh.PortForward{{RemoteAddr: "db.internal", RemotePort: 5432}},
		OnCreate: func(ctx context.Context, gotClient kubernetes.Client, pod *corev1.Pod) error {
			hookCalled = true

			if gotClient != client || ctx.Err() != nil {
				t.Fatal("hook did not receive the active client and context")
			}

			if len(clientset.Actions()) != 0 {
				t.Fatal("pod creation started before OnCreate")
			}

			if pod.Namespace != "default" || !strings.HasPrefix(pod.Name, "loop-tunnel-") || pod.Spec.Containers[0].Image != tunnelImage {
				t.Fatalf("hook received an incomplete pod: %+v", pod)
			}

			pod.Namespace = "custom"
			pod.Name = "custom-tunnel"
			pod.Spec.ImagePullSecrets = pullSecrets
			pod.Spec.Containers[0].Image = "registry.example.com/tunnel:custom"
			pod.Spec.ServiceAccountName = "tunnel-account"
			return nil
		},
	})

	if !errors.Is(err, waitErr) {
		t.Fatalf("Run error = %v, want %v", err, waitErr)
	}

	if !hookCalled || !waitCalled {
		t.Fatalf("OnCreate called = %v, WaitForPod called = %v", hookCalled, waitCalled)
	}

	if _, err := clientset.CoreV1().Pods("custom").Get(ctx, "custom-tunnel", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("customized pod was not cleaned up: %v", err)
	}
}

func TestRunOnCreateError(t *testing.T) {
	clientset := fake.NewClientset()
	client := &podTestClient{core: clientset.CoreV1()}
	hookErr := errors.New("cannot customize pod")

	err := Run(context.Background(), client, &RunOptions{
		Forwards: []ssh.PortForward{{RemoteAddr: "db.internal", RemotePort: 5432}},
		OnCreate: func(context.Context, kubernetes.Client, *corev1.Pod) error {
			return hookErr
		},
	})

	if !errors.Is(err, hookErr) {
		t.Fatalf("Run error = %v, want %v", err, hookErr)
	}

	if actions := clientset.Actions(); len(actions) != 0 {
		t.Fatalf("unexpected Kubernetes actions after hook failure: %v", actions)
	}
}

func TestRunOnCreateDoesNotDeleteExistingPod(t *testing.T) {
	ctx := context.Background()
	existing := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "existing"}}
	clientset := fake.NewClientset(existing)
	client := &podTestClient{core: clientset.CoreV1()}

	err := Run(ctx, client, &RunOptions{
		Forwards: []ssh.PortForward{{RemoteAddr: "db.internal", RemotePort: 5432}},
		OnCreate: func(_ context.Context, _ kubernetes.Client, pod *corev1.Pod) error {
			pod.Name = existing.Name
			return nil
		},
	})

	if !apierrors.IsAlreadyExists(err) {
		t.Fatalf("Run error = %v, want AlreadyExists", err)
	}

	if _, err := clientset.CoreV1().Pods(existing.Namespace).Get(ctx, existing.Name, metav1.GetOptions{}); err != nil {
		t.Fatalf("existing pod was removed after creation failed: %v", err)
	}
}

func TestRunViaExecSkipsOnCreate(t *testing.T) {
	clientset := fake.NewClientset()
	client := &podTestClient{core: clientset.CoreV1()}

	err := Run(context.Background(), client, &RunOptions{
		Pod:      "missing",
		Forwards: []ssh.PortForward{{RemoteAddr: "db.internal", RemotePort: 5432}},
		OnCreate: func(context.Context, kubernetes.Client, *corev1.Pod) error {
			t.Fatal("OnCreate must not run when tunneling through an existing pod")
			return nil
		},
	})

	if !apierrors.IsNotFound(err) {
		t.Fatalf("Run error = %v, want NotFound", err)
	}
}
