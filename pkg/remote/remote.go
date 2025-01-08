package remote

// import (
// 	"context"
// 	"fmt"
// 	"log"

// 	"github.com/adrianliechti/loop/pkg/cli"
// 	"github.com/adrianliechti/loop/pkg/kubernetes"
// 	"github.com/adrianliechti/loop/pkg/sftp"
// 	"github.com/adrianliechti/loop/pkg/ssh"
// 	"github.com/adrianliechti/loop/pkg/system"
// 	"github.com/adrianliechti/loop/pkg/to"

// 	corev1 "k8s.io/api/core/v1"
// )

// func UpdatePod(pod *corev1.Pod, root string) error {
// 	if root == "" {
// 		root = "/mnt"
// 	}

// 	volume := corev1.Volume{
// 		Name: "loop-volume",

// 		VolumeSource: corev1.VolumeSource{
// 			EmptyDir: &corev1.EmptyDirVolumeSource{},
// 		},
// 	}

// 	pod.Spec.Volumes = append([]corev1.Volume{volume}, pod.Spec.Volumes...)

// 	mount := corev1.VolumeMount{
// 		Name:             volume.Name,
// 		MountPropagation: to.Ptr(corev1.MountPropagationHostToContainer),

// 		MountPath: root,
// 	}

// 	for i, c := range pod.Spec.Containers {
// 		mounts := append([]corev1.VolumeMount{mount}, c.VolumeMounts...)
// 		pod.Spec.Containers[i].VolumeMounts = mounts

// 	}

// 	for i, c := range pod.Spec.InitContainers {
// 		mounts := append([]corev1.VolumeMount{mount}, c.VolumeMounts...)
// 		pod.Spec.InitContainers[i].VolumeMounts = mounts
// 	}

// 	container := corev1.Container{
// 		Name: "loop-tunnel",

// 		Image:           "ghcr.io/adrianliechti/loop-tunnel",
// 		ImagePullPolicy: corev1.PullAlways,

// 		SecurityContext: &corev1.SecurityContext{
// 			Privileged: to.Ptr(true),
// 		},

// 		VolumeMounts: []corev1.VolumeMount{
// 			{
// 				Name:             volume.Name,
// 				MountPropagation: to.Ptr(corev1.MountPropagationBidirectional),

// 				MountPath: "/mnt",
// 			},
// 		},
// 	}

// 	pod.Spec.Containers = append(pod.Spec.Containers, container)

// 	return nil
// }

// func Run(ctx context.Context, client kubernetes.Client, namespace, name string, path string, tunnels map[int]int) error {
// 	sshdport, err := system.FreePort(0)

// 	if err != nil {
// 		return err
// 	}

// 	sftpport, err := system.FreePort(0)

// 	if err != nil {
// 		return err
// 	}

// 	if err := startServer(ctx, sftpport, path); err != nil {
// 		return err
// 	}

// 	options := []ssh.Option{
// 		ssh.WithCommand("mkdir -p /mnt/src && sshfs -o allow_other -p 2222 root@localhost:/ /mnt/src && /bin/sleep infinity"),
// 	}

// 	if sftpport > 0 {
// 		options = append(options, ssh.WithRemotePortForward(ssh.PortForward{LocalPort: sftpport, RemotePort: 2222}))
// 	}

// 	for s, t := range tunnels {
// 		options = append(options, ssh.WithLocalPortForward(ssh.PortForward{LocalPort: s, RemotePort: t}))
// 	}

// 	c := ssh.New(fmt.Sprintf("127.0.0.1:%d", sshdport), options...)

// 	ready := make(chan struct{})

// 	go func() {
// 		<-ready

// 		if err := c.Run(ctx); err != nil {
// 			cli.Error(err)
// 		}
// 	}()

// 	if err := client.PodPortForward(ctx, namespace, name, "", map[int]int{sshdport: 22}, ready); err != nil {
// 		return err
// 	}

// 	return nil
// }

// func startServer(ctx context.Context, port int, path string) error {
// 	s := sftp.NewServer(fmt.Sprintf("127.0.0.1:%d", port), path)

// 	go func() {
// 		<-ctx.Done()
// 		s.Close()
// 	}()

// 	go func() {
// 		if err := s.ListenAndServe(); err != nil {
// 			log.Println("could not start server", "error", err)
// 		}
// 	}()

// 	return nil
// }
