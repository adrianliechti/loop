package os

import (
	"os"
	"path/filepath"
	"time"

	"github.com/adrianliechti/loop/pkg/fs"
)

var _ fs.FS = &FS{}

type FS struct {
}

func NewFS() (*FS, error) {
	return &FS{}, nil
}

func NewWatchableFS() (fs.WatchableFS, error) {
	fs, err := NewFS()

	if err != nil {
		return nil, err
	}

	watcher, err := NewWatcher()

	if err != nil {
		return nil, err
	}

	return &struct {
		*FS
		*Watcher
	}{fs, watcher}, nil
}

func (fs *FS) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func (fs *FS) Lstat(path string) (fs.FileInfo, error) {
	return os.Lstat(path)
}

func (fs *FS) Open(path string) (fs.File, error) {
	return os.Open(path)
}

func (fs *FS) Create(path string) (fs.File, error) {
	return os.Create(path)
}

func (fs *FS) Mkdir(path string, mode fs.FileMode) error {
	return os.Mkdir(path, mode)
}

func (fs *FS) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}

func (fs *FS) Rename(oldname, newpath string) error {
	return os.Rename(oldname, newpath)
}

func (fs *FS) Remove(path string) error {
	return os.Remove(path)
}

func (fs *FS) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (fs *FS) Chown(path string, uid, gid int) error {
	return os.Chown(path, uid, gid)
}

func (fs *FS) Chmod(path string, mode fs.FileMode) error {
	return os.Chmod(path, mode)
}

func (fs *FS) Chtimes(path string, atime time.Time, mtime time.Time) error {
	return os.Chtimes(path, atime, mtime)
}

func (fs *FS) WalkDir(path string, fn fs.WalkDirFunc) error {
	return filepath.WalkDir(path, fn)
}
