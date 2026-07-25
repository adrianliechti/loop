package kubernetes

import (
	"context"
	"fmt"
	"io"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/util/term"
)

// podStream backs PodExec and PodAttach, which drive the two pod subresources
// that stream stdio over SPDY and differ only in their request options. An
// empty container defaults to the pod's first one. With tty set the local
// terminal is put in raw mode and its size is mirrored to the remote process.
func (c *client) podStream(ctx context.Context, namespace, name, container, subresource string, tty bool, stdin io.Reader, stdout, stderr io.Writer, options func(container string) runtime.Object) error {
	if container == "" {
		pod, err := c.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})

		if err != nil {
			return err
		}

		if len(pod.Spec.Containers) == 0 {
			return fmt.Errorf("pod %s/%s has no containers", namespace, name)
		}

		container = pod.Spec.Containers[0].Name
	}

	req := c.CoreV1().RESTClient().Post().Resource("pods").Name(name).Namespace(namespace).SubResource(subresource)

	req.VersionedParams(options(container), scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.Config(), "POST", req.URL())

	if err != nil {
		return err
	}

	streamOptions := remotecommand.StreamOptions{
		Tty: tty,

		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	}

	if !tty {
		return exec.StreamWithContext(ctx, streamOptions)
	}

	t := term.TTY{
		Out: os.Stdout,
		In:  os.Stdin,
		Raw: true,
	}

	streamOptions.TerminalSizeQueue = &terminalSizeQueueAdapter{
		delegate: t.MonitorSize(t.GetSize()),
	}

	return t.Safe(func() error {
		return exec.StreamWithContext(ctx, streamOptions)
	})
}
