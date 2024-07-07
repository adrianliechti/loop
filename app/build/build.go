package build

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/adrianliechti/loop/app"
	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/to"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/uuid"
	"github.com/moby/moby/pkg/archive"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	container = "buildkitd"
)

var Command = &cli.Command{
	Name:  "build",
	Usage: "build image with BuildKit",

	Flags: []cli.Flag{
		app.NamespaceFlag,

		&cli.StringFlag{
			Name:  "image",
			Usage: "image name (format: registry/repository:tag)",
		},

		&cli.BoolFlag{
			Name:  "insecure",
			Usage: "use insecure registry",
		},

		&cli.StringFlag{
			Name:  "username",
			Usage: "registry username",
		},

		&cli.StringFlag{
			Name:  "password",
			Usage: "registry password",
		},
	},

	Action: func(c *cli.Context) error {
		client := app.MustClient(c)

		path, err := ParsePath(c.Args().Get(0))

		if err != nil {
			return err
		}

		image, err := ParseImage(c.String("image"))

		if err != nil {
			return err
		}

		image.Insecure = c.Bool("insecure")

		image.Username = c.String("username")
		image.Password = c.String("password")

		return Run(c.Context, client, "", image, path, "")
	},
}

type Image struct {
	Name string
	Tag  string

	Insecure bool
	Registry string

	Username string
	Password string
}

func (i *Image) String() string {
	s := i.Name

	if i.Registry != "" {
		s = i.Registry + "/" + s
	}

	if i.Tag != "" {
		s = s + ":" + i.Tag
	}

	s = strings.TrimPrefix(s, "index.docker.io/")

	return s
}

func ParsePath(path string) (string, error) {
	if path == "" || path == "." {
		return os.Getwd()
	}

	return filepath.Abs(path)
}

func ParseImage(image string) (Image, error) {
	tag, err := name.NewTag(image)

	if err != nil {
		return Image{}, err
	}

	return Image{
		Name: tag.RepositoryStr(),
		Tag:  tag.TagStr(),

		Registry: tag.RegistryStr(),
	}, nil
}

func Run(ctx context.Context, client kubernetes.Client, namespace string, image Image, dir, dockerfile string) error {
	if namespace == "" {
		namespace = client.Namespace()
	}

	if dir == "" || dir == "." {
		wd, err := os.Getwd()

		if err != nil {
			return err
		}

		dir = wd
	}

	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}

	f, err := archive.TarWithOptions(dir, &archive.TarOptions{})

	if err != nil {
		return err
	}

	name := "loop-buildkit-" + uuid.New().String()[0:7]

	cli.Infof("Starting BuildKit pod (%s/%s)...", namespace, name)
	pod, err := startPod(ctx, client, namespace, name, "")

	if err != nil {
		return err
	}

	defer func() {
		cli.Infof("Stopping BuildKit pod (%s/%s)...", pod.Namespace, pod.Name)
		stopPod(context.Background(), client, pod.Namespace, pod.Name)
	}()

	cli.Infof("Copy Context...")

	builderPath := "/tmp/build-" + uuid.New().String()
	builderContext := builderPath
	builderDockerfile := path.Dir(path.Join(builderPath, dockerfile))

	if err := client.PodExec(ctx, pod.Namespace, pod.Name, container, []string{"mkdir", "-p", builderPath}, false, nil, io.Discard, io.Discard); err != nil {
		return err
	}

	if err := client.PodExec(ctx, namespace, name, container, []string{"tar", "xf", "-", "-C", builderPath}, false, f, io.Discard, io.Discard); err != nil {
		return err
	}

	if image.Username != "" && image.Password != "" {
		registry := image.Registry

		if registry == "index.docker.io" || registry == "" {
			registry = "https://index.docker.io/v1/"
		}

		config := docker.ConfigFile{
			AuthConfigs: map[string]docker.AuthConfig{
				registry: {
					Username: image.Username,
					Password: image.Password,

					Auth: base64.StdEncoding.EncodeToString([]byte(image.Username + ":" + image.Password)),
				},
			},
		}

		data, _ := json.Marshal(config)

		if err := client.PodExec(ctx, pod.Namespace, pod.Name, container, []string{"mkdir", "-p", "/root/.docker"}, false, nil, io.Discard, io.Discard); err != nil {
			return err
		}

		if err := client.PodExec(ctx, namespace, name, container, []string{"cp", "/dev/stdin", "/root/.docker/config.json"}, false, bytes.NewReader(data), io.Discard, io.Discard); err != nil {
			return err
		}
	}

	output := []string{
		"type=image",
		"push=true",
		"name=" + image.String(),
	}

	if image.Insecure {
		output = append(output, "registry.insecure=true")
	}

	build := []string{
		"buildctl",
		"build",

		"--frontend", "dockerfile.v0",

		"--local", "context=" + builderContext,
		"--local", "dockerfile=" + builderDockerfile,

		"--output", strings.Join(output, ","),
	}

	if err := client.PodExec(ctx, namespace, name, container, build, false, f, os.Stdout, os.Stderr); err != nil {
		return err
	}

	return nil
}

func startPod(ctx context.Context, client kubernetes.Client, namespace, name, image string) (*corev1.Pod, error) {
	if image == "" {
		image = "moby/buildkit"
	}

	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"buildctl",
					"debug",
					"workers",
				},
			},
		},

		InitialDelaySeconds: 5,
		PeriodSeconds:       10,
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},

		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: container,

					Image:           image,
					ImagePullPolicy: corev1.PullAlways,

					SecurityContext: &corev1.SecurityContext{
						Privileged: to.Ptr(true),
					},

					ReadinessProbe: probe,
					LivenessProbe:  probe,
				},
			},

			TerminationGracePeriodSeconds: to.Ptr(int64(10)),
		},
	}

	if _, err := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return nil, err
	}

	return client.WaitForPod(ctx, namespace, name)
}

func stopPod(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	return client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: to.Ptr(int64(0)),
	})
}
