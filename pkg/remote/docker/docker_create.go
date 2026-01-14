package docker

import (
	"context"

	"github.com/adrianliechti/go-cli"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/google/uuid"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CreateOptions struct {
	Name      string
	Namespace string

	Image       string
	TunnelImage string

	CPU     resource.Quantity
	Memory  resource.Quantity
	Storage resource.Quantity

	OnCreate func(ctx context.Context, client kubernetes.Client, statefulset *appsv1.StatefulSet) error
	OnReady  func(ctx context.Context, client kubernetes.Client, statefulset *appsv1.StatefulSet) error
}

func Create(ctx context.Context, client kubernetes.Client, options *CreateOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if options == nil {
		options = new(CreateOptions)
	}

	if options.Name == "" {
		options.Name = uuid.NewString()[0:7]
	}

	if options.Namespace == "" {
		options.Namespace = client.Namespace()
	}

	if options.Image == "" {
		options.Image = "public.ecr.aws/docker/library/docker:29-dind"
	}

	if options.TunnelImage == "" {
		options.TunnelImage = "ghcr.io/adrianliechti/loop-tunnel"
	}

	if options.Storage.IsZero() {
		options.Storage = resource.MustParse("10Gi")
	}

	statefulset := templateStatefulSet(options)

	if options.OnCreate != nil {
		if err := options.OnCreate(ctx, client, statefulset); err != nil {
			return err
		}
	}

	cli.Infof("★ Creating daemon (%s/%s)...", statefulset.Namespace, options.Name)

	if err := createStatefulSet(ctx, client, statefulset); err != nil {
		return err
	}

	if options.OnReady != nil {
		if err := options.OnReady(ctx, client, statefulset); err != nil {
			return err
		}
	}

	return nil
}

func templateStatefulSet(options *CreateOptions) *appsv1.StatefulSet {
	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName(options.Name),
			Namespace: options.Namespace,

			Labels: resourceLabels(options.Name),
		},

		Spec: appsv1.StatefulSetSpec{
			ServiceName: resourceName(options.Name),

			Selector: &metav1.LabelSelector{
				MatchLabels: resourceLabels(options.Name),
			},

			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: resourceLabels(options.Name),
				},

				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "docker",

							Image:           options.Image,
							ImagePullPolicy: corev1.PullAlways,

							SecurityContext: &corev1.SecurityContext{
								Privileged: kubernetes.Ptr(true),
							},

							Env: []corev1.EnvVar{
								{
									Name:  "DOCKER_HOST",
									Value: "tcp://127.0.0.1:2375",
								},
								{
									Name:  "DOCKER_TLS_CERTDIR",
									Value: "",
								},
							},

							Args: []string{
								"--tls=false",
							},

							Ports: []corev1.ContainerPort{
								{
									Name:          "docker",
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: int32(2375),
								},
							},

							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "docker",
									MountPath: "/var/lib/docker",
								},
								{
									Name:      "modules",
									MountPath: "/lib/modules",
									ReadOnly:  true,
								},
								{
									Name:             "loop-data",
									MountPath:        "/data",
									MountPropagation: kubernetes.Ptr(corev1.MountPropagationHostToContainer),
								},
							},
						},
						{
							Name: "loop-tunnel",

							Image:           options.TunnelImage,
							ImagePullPolicy: corev1.PullAlways,

							SecurityContext: &corev1.SecurityContext{
								Privileged: kubernetes.Ptr(true),
							},

							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},

								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
							},

							VolumeMounts: []corev1.VolumeMount{
								{
									Name:             "loop-data",
									MountPath:        "/data",
									MountPropagation: kubernetes.Ptr(corev1.MountPropagationBidirectional),
								},
							},
						},
					},

					Volumes: []corev1.Volume{
						{
							Name: "modules",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/lib/modules",
								},
							},
						},
						{
							Name: "loop-data",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},

					TerminationGracePeriodSeconds: kubernetes.Ptr(int64(10)),
				},
			},

			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "docker",
					},

					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},

						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: options.Storage,
							},
						},
					},
				},
			},
		},
	}

	return statefulset
}

func createStatefulSet(ctx context.Context, client kubernetes.Client, statefulset *appsv1.StatefulSet) error {
	statefulset, err := client.AppsV1().StatefulSets(statefulset.Namespace).Create(ctx, statefulset, metav1.CreateOptions{})

	if err != nil {
		return err
	}

	pod := statefulset.Name + "-0"

	if _, err := client.WaitForPod(ctx, statefulset.Namespace, pod); err != nil {
		return err
	}

	return nil
}
