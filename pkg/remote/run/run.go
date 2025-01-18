package run

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/fs"
	"github.com/adrianliechti/loop/pkg/kubernetes"
	"github.com/adrianliechti/loop/pkg/ssh"
	"github.com/adrianliechti/loop/pkg/system"
	"github.com/adrianliechti/loop/pkg/to"

	"github.com/google/uuid"

	osfs "github.com/adrianliechti/loop/pkg/fs/os"
	sftpfs "github.com/adrianliechti/loop/pkg/fs/sftp"
	gossh "golang.org/x/crypto/ssh"

	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/idtools"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Container struct {
	Image string

	Identity *Identity

	TTY bool

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	Ports   []Port
	Volumes []Volume
}

type Identity struct {
	UID int
	GID int
}

type Port struct {
	Source int
	Target int
}

type Volume struct {
	Source string
	Target string

	Identity *Identity
}

type SyncMode string

const (
	SyncModeNone SyncMode = ""
	SyncLocal    SyncMode = "local"
	SyncRemote   SyncMode = "remote"
)

type RunOptions struct {
	Name      string
	Namespace string

	// Loop Tunnel image overwrite
	Image string

	SyncMode SyncMode

	OnPod   func(ctx context.Context, client kubernetes.Client, pod *corev1.Pod) error
	OnReady func(ctx context.Context, client kubernetes.Client, pod *corev1.Pod) error
}

func Run(ctx context.Context, client kubernetes.Client, container *Container, options *RunOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if options == nil {
		options = new(RunOptions)
	}

	if options.Name == "" {
		options.Name = "loop-run-" + uuid.NewString()[0:7]
	}

	if options.Namespace == "" {
		options.Namespace = client.Namespace()
	}

	if options.Image == "" {
		options.Image = "ghcr.io/adrianliechti/loop-tunnel"
	}

	pod := templatePod(ctx, client, container, options)

	if options.OnPod != nil {
		if err := options.OnPod(ctx, client, pod); err != nil {
			return err
		}
	}

	cli.Infof("★ creating container (%s/%s)...", pod.Namespace, pod.Name)

	if err := startPod(ctx, client, pod); err != nil {
		return err
	}

	defer func() {
		cli.Infof("★ removing container (%s/%s)...", pod.Namespace, pod.Name)
		stopPod(context.Background(), client, pod.Namespace, pod.Name)
	}()

	cli.Infof("★ copying volumes data...")

	if err := copyVolumes(ctx, client, pod.Namespace, pod.Name, container.Volumes); err != nil {
		return err
	}

	go func() {
		connectTunnel(ctx, client, pod, container, options)
		cancel()
	}()

	if options.OnReady != nil {
		if err := options.OnReady(ctx, client, pod); err != nil {
			return err
		}
	}

	return client.PodAttach(ctx, pod.Namespace, pod.Name, "main", container.TTY, container.Stdin, container.Stdout, container.Stderr)
}

func templatePod(ctx context.Context, client kubernetes.Client, container *Container, options *RunOptions) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.Name,
			Namespace: options.Namespace,
		},

		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",

					Image:           container.Image,
					ImagePullPolicy: corev1.PullAlways,

					TTY:   container.TTY,
					Stdin: container.Stdin != nil,
				},
			},

			TerminationGracePeriodSeconds: to.Ptr(int64(10)),
		},
	}

	if len(container.Volumes) == 0 && len(container.Ports) == 0 {
		return pod
	}

	volume := corev1.Volume{
		Name: "loop-data",

		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}

	pod.Spec.Volumes = append(pod.Spec.Volumes, volume)

	var mounts []corev1.VolumeMount

	for _, v := range container.Volumes {
		mount := corev1.VolumeMount{
			Name: volume.Name,

			MountPath: v.Target,
			SubPath:   strings.TrimLeft(v.Target, "/"),
		}

		mounts = append(mounts, mount)
	}

	for i, c := range pod.Spec.InitContainers {
		pod.Spec.InitContainers[i].VolumeMounts = append(c.VolumeMounts, mounts...)
	}

	for i, c := range pod.Spec.Containers {
		pod.Spec.Containers[i].VolumeMounts = append(c.VolumeMounts, mounts...)
	}

	pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
		Name: "loop-tunnel",

		Image:           options.Image,
		ImagePullPolicy: corev1.PullAlways,

		VolumeMounts: []corev1.VolumeMount{
			{
				Name: volume.Name,

				MountPath: "/data",
			},
		},
	})

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
		GracePeriodSeconds: to.Ptr(int64(0)),
	})
}

func connectTunnel(ctx context.Context, client kubernetes.Client, pod *corev1.Pod, container *Container, options *RunOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	port, err := system.FreePort(0)

	if err != nil {
		return err
	}

	ready := make(chan struct{})

	go func() {
		client.PodPortForward(ctx, pod.Namespace, pod.Name, "", map[int]int{port: 22}, ready)
		cancel()
	}()

	<-ready

	addr := fmt.Sprintf("127.0.0.1:%d", port)

	if len(container.Ports) > 0 {
		go func() {
			if err := tunnelPorts(ctx, addr, container.Ports); err != nil {
				cli.Error(err)
			}

			cancel()
		}()
	}

	if len(container.Volumes) > 0 {
		config := &gossh.ClientConfig{
			User: "root",

			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		}

		conn, err := gossh.Dial("tcp", addr, config)

		if err != nil {
			return err
		}

		local, err := osfs.NewWatchableFS()

		if err != nil {
			return err
		}

		remote, err := sftpfs.NewWatchableFS(conn)

		if err != nil {
			return err
		}

		if options.SyncMode == SyncLocal {
			go func() {
				if err := syncLocalChanges(ctx, local, remote, container.Volumes); err != nil {
					cli.Error(err)
				}

				cancel()
			}()
		}

		if options.SyncMode == SyncRemote {
			go func() {
				if err := syncRemoteVolumes(ctx, local, remote, container.Volumes); err != nil {
					cli.Error(err)
				}

				cancel()
			}()
		}
	}

	<-ctx.Done()

	return nil
}

func tunnelPorts(ctx context.Context, addr string, ports []Port) error {
	var options []ssh.Option

	for _, p := range ports {
		options = append(options, ssh.WithLocalPortForward(ssh.PortForward{LocalPort: p.Source, RemotePort: p.Target}))
	}

	client := ssh.New(addr, options...)

	return client.Run(ctx)
}

func copyVolumes(ctx context.Context, client kubernetes.Client, namespace, name string, volumes []Volume) error {
	for _, v := range volumes {
		path := path.Join("/data", v.Target)

		options := &archive.TarOptions{}

		if v.Identity != nil {
			options.ChownOpts = &idtools.Identity{
				UID: v.Identity.UID,
				GID: v.Identity.GID,
			}
		}

		tar, err := archive.TarWithOptions(v.Source, options)

		if err != nil {
			return err
		}

		if err := client.PodExec(ctx, namespace, name, "loop-tunnel", []string{"tar", "xf", "-", "-C", path}, false, tar, os.Stdout, os.Stdout); err != nil {
			return err
		}
	}

	return nil
}

func syncLocalChanges(ctx context.Context, local, remote fs.WatchableFS, volumes []Volume) error {
	root := "/data"

	var paths []string

	for _, v := range volumes {
		paths = append(paths, v.Source)
	}

	events, err := local.Watch(ctx, paths...)

	if err != nil {
		return err
	}

	for e := range events {
		if ignoredChange(e.Path) {
			continue
		}

		remotePath := path.Join(root, mapRemotePath(volumes, e.Path))

		if remotePath == "" {
			continue
		}

		// println(e.Action, e.Path, remotePath)

		switch e.Action {
		case fs.Create, fs.Modify:
			info, err := local.Stat(e.Path)

			if err != nil {
				continue
			}

			if r := path.Join(root, mapRemotePath(volumes, e.RenamedFrom)); r != root {
				if _, err := remote.Stat(r); err == nil {
					if err := remote.Rename(r, remotePath); err != nil {
						cli.Error("rename remote dir", err)
						continue
					}

					continue
				}
			}

			if i, err := remote.Stat(remotePath); err == nil {
				if i.ModTime().After(info.ModTime()) || i.ModTime().Equal(info.ModTime()) {
					continue
				}
			}

			if info.IsDir() {
				if err := syncDir(ctx, remote, remotePath, local, e.Path); err != nil {
					cli.Error("create remote dir", err)
					continue
				}

				continue
			}

			if err := syncFile(ctx, remote, remotePath, local, e.Path); err != nil {
				cli.Error("create remote file", err)
				continue
			}

		case fs.Remove:
			info, err := remote.Stat(remotePath)

			if err != nil {
				continue
			}

			if info.IsDir() {
				if err := remote.RemoveAll(remotePath); err != nil {
					cli.Error("remove remote dir", err)
					continue
				}

				continue
			}

			if err := remote.Remove(remotePath); err != nil {
				cli.Error("remove remote file", err)
				continue
			}
		}
	}

	return nil
}

func syncRemoteVolumes(ctx context.Context, local, remote fs.WatchableFS, volumes []Volume) error {
	root := "/data"

	var paths []string

	for _, v := range volumes {
		paths = append(paths, path.Join("/data", v.Target))
	}

	events, err := remote.Watch(ctx, paths...)

	if err != nil {
		return err
	}

	for e := range events {
		if ignoredChange(e.Path) {
			continue
		}

		localPath := mapLocalPath(volumes, strings.TrimPrefix(e.Path, root))

		if localPath == "" {
			continue
		}

		// println(e.Action, e.Path, localPath)

		switch e.Action {
		case fs.Create, fs.Modify:
			info, err := remote.Stat(e.Path)

			if err != nil {
				continue
			}

			if l := mapLocalPath(volumes, strings.TrimPrefix(e.RenamedFrom, root)); l != "" {
				if _, err := local.Stat(l); err == nil {
					if err := local.Rename(l, localPath); err != nil {
						cli.Error("remove local file", err)
						continue
					}

					continue
				}
			}

			if i, err := local.Stat(localPath); err == nil {
				if i.ModTime().After(info.ModTime()) || i.ModTime().Equal(info.ModTime()) {
					continue
				}
			}

			if info.IsDir() {
				if err := syncDir(ctx, local, localPath, remote, e.Path); err != nil {
					cli.Error("create local dir", err)
					continue
				}

				continue
			}

			if err := syncFile(ctx, local, localPath, remote, e.Path); err != nil {
				cli.Error("create local file", err)
				continue
			}

		case fs.Remove:
			info, err := local.Stat(localPath)

			if err != nil {
				continue
			}

			if info.IsDir() {
				if err := local.RemoveAll(localPath); err != nil {
					cli.Error("delete local dir", err)
					continue
				}

				continue
			}

			if err := local.Remove(localPath); err != nil {
				cli.Error("delete local file", err)
				continue
			}
		}
	}

	return nil
}

func syncFile(ctx context.Context, src fs.FS, srcPath string, dst fs.FS, dstPath string) error {
	tmp := path.Join(path.Dir(srcPath), uuid.NewString()+".tmp")

	t, err := src.Create(tmp)

	if err != nil {
		return err
	}

	defer src.Remove(tmp)

	defer t.Close()

	f, err := dst.Open(dstPath)

	if err != nil {
		return err
	}

	i, err := f.Stat()

	if err != nil {
		return err
	}

	defer f.Close()

	if _, err := io.Copy(t, f); err != nil {
		return err
	}

	t.Close()
	f.Close()

	os.Chtimes(t.Name(), i.ModTime(), i.ModTime())

	dir := path.Dir(t.Name())
	src.MkdirAll(dir, 0755)

	return src.Rename(t.Name(), srcPath)
}

func relPath(base, targpath string) string {
	base = path.Clean(filepath.ToSlash(base))

	targpath = path.Clean(filepath.ToSlash(targpath))
	targpath = strings.TrimLeft(targpath, "/")

	rel := strings.TrimPrefix(targpath, base)
	rel = strings.TrimLeft(rel, "/")

	return rel
}

func syncDir(ctx context.Context, src fs.FS, srcPath string, dst fs.FS, dstPath string) error {
	return dst.WalkDir(dstPath, func(dirPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel := relPath(dstPath, dirPath)
		name := path.Join(srcPath, rel)

		i, err := d.Info()

		if err != nil {
			return err
		}

		if d.IsDir() {
			if err := src.MkdirAll(name, i.Mode()); err != nil {
				return err
			}

			src.Chtimes(name, i.ModTime(), i.ModTime())

			return nil
		}

		return syncFile(ctx, src, name, dst, dirPath)
	})
}

func mapLocalPath(volumes []Volume, remotePath string) string {
	if remotePath == "" {
		return ""
	}

	longestMatch := ""
	longestValue := ""

	for _, v := range volumes {
		if strings.HasPrefix(remotePath, v.Target) && len(v.Target) > len(longestMatch) {
			longestMatch = v.Target
			longestValue = v.Source
		}
	}

	rel, _ := filepath.Rel(longestMatch, filepath.FromSlash(remotePath))
	return filepath.Join(longestValue, rel)
}

func mapRemotePath(volumes []Volume, localPath string) string {
	if localPath == "" {
		return ""
	}

	longestMatch := ""
	longestValue := ""

	for _, v := range volumes {
		if strings.HasPrefix(localPath, v.Source) && len(v.Source) > len(longestMatch) {
			longestMatch = v.Source
			longestValue = v.Target
		}
	}

	rel := relPath(longestMatch, filepath.ToSlash(localPath))
	return path.Join(longestValue, rel)
}

func ignoredChange(name string) bool {
	ext := path.Ext(name)

	if ext == ".tmp" {
		return true
	}

	return false
}
