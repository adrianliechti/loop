package kubernetes

import (
	"context"
	"io"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/util/term"
)

func (c *client) PodExec(ctx context.Context, namespace, name, container string, command []string, tty bool, stdin io.Reader, stdout, stderr io.Writer) error {
	if container == "" {
		pod, err := c.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})

		if err != nil {
			return err
		}

		container = pod.Spec.Containers[0].Name
	}

	req := c.CoreV1().RESTClient().Post().Resource("pods").Name(name).Namespace(namespace).SubResource("exec")

	req.VersionedParams(
		&corev1.PodExecOptions{
			Container: container,
			TTY:       tty,

			Stdin:  stdin != nil,
			Stdout: stdout != nil,
			Stderr: stderr != nil,

			Command: command,
		},
		scheme.ParameterCodec,
	)

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

	if tty {
		t := term.TTY{
			Out: os.Stdout,
			In:  os.Stdin,
			Raw: true,
		}

		streamOptions.TerminalSizeQueue = t.MonitorSize(t.GetSize())

		return t.Safe(func() error {
			return exec.StreamWithContext(ctx, streamOptions)
		})
	}

	return exec.StreamWithContext(ctx, streamOptions)
}
