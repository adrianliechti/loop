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

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/docker"
	"github.com/adrianliechti/loop/pkg/kubernetes"

	"github.com/Masterminds/semver/v3"
	"github.com/google/uuid"

	"github.com/docker/docker/pkg/archive"
	"github.com/google/go-containerregistry/pkg/name"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	container = "buildkitd"
)

type Image struct {
	Name string
	Tag  string

	Insecure bool
	Registry string

	Username string
	Password string
}

type RunOptions struct {
	Name      string
	Namespace string

	// Rootless BuildKit image overwrite
	Image string

	Stdout io.Writer
	Stderr io.Writer

	OnPod     func(ctx context.Context, client kubernetes.Client, pod *corev1.Pod) error
	OnContext func(ctx context.Context, client kubernetes.Client, pod *corev1.Pod, path string) error
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

func Run(ctx context.Context, client kubernetes.Client, image Image, dir, file string, options *RunOptions) error {
	if options == nil {
		options = new(RunOptions)
	}

	if options.Name == "" {
		options.Name = "loop-build-" + uuid.NewString()[0:7]
	}

	if options.Namespace == "" {
		options.Namespace = client.Namespace()
	}

	if options.Image == "" {
		options.Image = "moby/buildkit:rootless"
	}

	if options.Stdout == nil {
		options.Stdout = os.Stdout
	}

	if options.Stderr == nil {
		options.Stderr = os.Stderr
	}

	if dir == "" || dir == "." {
		wd, err := os.Getwd()

		if err != nil {
			return err
		}

		dir = wd
	}

	if file == "" {
		file = "Dockerfile"
	}

	f, err := archive.TarWithOptions(dir, &archive.TarOptions{})

	if err != nil {
		return err
	}

	pod := templatePod(ctx, client, options)

	if options.OnPod != nil {
		if err := options.OnPod(ctx, client, pod); err != nil {
			return err
		}
	}

	cli.Infof("★ creating container (%s/%s)...", pod.Namespace, pod.Name)

	defer func() {
		cli.Infof("★ removing container (%s/%s)...", pod.Namespace, pod.Name)
		stopPod(context.Background(), client, pod.Namespace, pod.Name)
	}()

	if err := startPod(ctx, client, pod); err != nil {
		return err
	}

	cli.Infof("★ copying build context...")

	builderPath := "/data/build-" + uuid.NewString()[0:7]
	builderContext := builderPath
	builderDockerfile := path.Dir(path.Join(builderPath, file))

	if err := client.PodExec(ctx, pod.Namespace, pod.Name, container, []string{"mkdir", "-p", builderPath}, false, nil, options.Stdout, options.Stderr); err != nil {
		return err
	}

	if err := client.PodExec(ctx, pod.Namespace, pod.Name, container, []string{"tar", "xf", "-", "-C", builderPath}, false, f, options.Stdout, options.Stderr); err != nil {
		return err
	}

	if options.OnContext != nil {
		if err := options.OnContext(ctx, client, pod, builderPath); err != nil {
			return err
		}
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

		if err := client.PodExec(ctx, pod.Namespace, pod.Name, container, []string{"mkdir", "-p", "/home/user/.docker"}, false, nil, options.Stdout, options.Stderr); err != nil {
			return err
		}

		if err := client.PodExec(ctx, pod.Namespace, pod.Name, container, []string{"cp", "/dev/stdin", "/home/user/.docker/config.json"}, false, bytes.NewReader(data), options.Stdout, options.Stderr); err != nil {
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

	if err := client.PodExec(ctx, pod.Namespace, pod.Name, container, build, false, f, options.Stdout, options.Stderr); err != nil {
		return err
	}

	return nil
}

func templatePod(ctx context.Context, client kubernetes.Client, options *RunOptions) *corev1.Pod {
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
			Name:      options.Name,
			Namespace: options.Namespace,
		},

		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: container,

					Image:           options.Image,
					ImagePullPolicy: corev1.PullAlways,

					Args: []string{
						"--oci-worker-no-process-sandbox",
					},

					SecurityContext: &corev1.SecurityContext{
						AppArmorProfile: &corev1.AppArmorProfile{
							Type: corev1.AppArmorProfileTypeUnconfined,
						},

						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeUnconfined,
						},

						RunAsUser:  kubernetes.Ptr(int64(1000)),
						RunAsGroup: kubernetes.Ptr(int64(1000)),
					},

					ReadinessProbe: probe,
					LivenessProbe:  probe,

					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "data",
							MountPath: "/data",
						},
						{
							Name:      "buildkit",
							MountPath: "/home/user/.local/share/buildkit",
						},
					},
				},
			},

			Volumes: []corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: "buildkit",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},

			TerminationGracePeriodSeconds: kubernetes.Ptr(int64(10)),
		},
	}

	if version, err := client.Version(ctx); err == nil {
		c, _ := semver.NewConstraint("< 1.30")
		if c.Check(version) {
			if pod.Annotations == nil {
				pod.Annotations = map[string]string{}
			}

			pod.Spec.Containers[0].SecurityContext.AppArmorProfile = nil
			pod.Annotations["container.apparmor.security.beta.kubernetes.io/buildkitd"] = "unconfined"
		}
	}

	return pod
}

func startPod(ctx context.Context, client kubernetes.Client, pod *corev1.Pod) error {
	pod, err := client.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})

	if err != nil {
		return err
	}

	if _, err := client.WaitForPod(ctx, pod.Namespace, pod.Name); err != nil {
		return err
	}

	return nil
}

func stopPod(ctx context.Context, client kubernetes.Client, namespace, name string) error {
	return client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: kubernetes.Ptr(int64(0)),
	})
}
