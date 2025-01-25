package sftp

import (
	"time"

	"github.com/adrianliechti/loop/pkg/fs"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

var _ fs.FS = &FS{}

type FS struct {
	conn   *ssh.Client
	client *sftp.Client
}

func NewFS(conn *ssh.Client) (*FS, error) {
	client, err := sftp.NewClient(conn)

	if err != nil {
		return nil, err
	}

	return &FS{conn: conn, client: client}, nil
}

func NewWatchableFS(conn *ssh.Client) (fs.WatchableFS, error) {
	fs, err := NewFS(conn)

	if err != nil {
		return nil, err
	}

	watcher, err := NewWatcher(conn)

	if err != nil {
		return nil, err
	}

	return &struct {
		*FS
		*Watcher
	}{fs, watcher}, nil
}

func (fs *FS) Stat(path string) (fs.FileInfo, error) {
	return fs.client.Stat(path)
}

func (fs *FS) Lstat(path string) (fs.FileInfo, error) {
	return fs.client.Lstat(path)
}

func (fs *FS) Open(path string) (fs.File, error) {
	return fs.client.Open(path)
}

func (fs *FS) Create(path string) (fs.File, error) {
	return fs.client.Create(path)
}

func (fs *FS) Mkdir(path string, mode fs.FileMode) error {
	if err := fs.client.Mkdir(path); err != nil {
		return err
	}

	if err := fs.client.Chmod(path, mode); err != nil {
		return err
	}

	return nil
}

func (fs *FS) MkdirAll(path string, mode fs.FileMode) error {
	if err := fs.Mkdir(path, mode); err != nil {
		return err
	}

	return fs.client.MkdirAll(path)
}

func (fs *FS) Rename(oldname, newpath string) error {
	return fs.client.PosixRename(oldname, newpath)
}

func (fs *FS) Remove(path string) error {
	return fs.client.Remove(path)
}

func (fs *FS) RemoveAll(path string) error {
	return fs.client.RemoveAll(path)
}

func (fs *FS) Chown(path string, uid, gid int) error {
	return fs.client.Chown(path, uid, gid)
}

func (fs *FS) Chmod(path string, mode fs.FileMode) error {
	return fs.client.Chmod(path, mode)
}

func (fs *FS) Chtimes(path string, atime time.Time, mtime time.Time) error {
	return fs.client.Chtimes(path, atime, mtime)
}

func (fs *FS) WalkDir(path string, fn fs.WalkDirFunc) error {
	walker := fs.client.Walk(path)

	for walker.Step() {
		err := walker.Err()

		name := walker.Path()
		info := walker.Stat()

		d := dirEntry{
			client: fs.client,

			name: name,
			info: info,
		}

		fn(name, d, err)
	}

	return nil
}

type dirEntry struct {
	client *sftp.Client

	name string
	info fs.FileInfo
}

func (d dirEntry) Name() string {
	return d.name
}

func (d dirEntry) Type() fs.FileMode {
	return d.info.Mode().Type()
}

func (d dirEntry) Info() (fs.FileInfo, error) {
	return d.info, nil
}

func (d dirEntry) IsDir() bool {
	return d.info.IsDir()
}
