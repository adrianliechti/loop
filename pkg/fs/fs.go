package fs

import (
	"io/fs"
	"time"
)

type File interface {
	Name() string
	Stat() (FileInfo, error)

	Read([]byte) (int, error)
	Write(b []byte) (n int, err error)
	Seek(offset int64, whence int) (ret int64, err error)

	Close() error
}

type FileMode = fs.FileMode
type FileInfo = fs.FileInfo

type DirEntry = fs.DirEntry
type WalkDirFunc = fs.WalkDirFunc

type FS interface {
	Stat(path string) (FileInfo, error)
	Lstat(path string) (fs.FileInfo, error)

	Open(path string) (File, error)
	Create(path string) (File, error)

	Mkdir(path string, mode FileMode) error
	MkdirAll(path string, mode FileMode) error

	Rename(oldpath, newpath string) error

	Remove(path string) error
	RemoveAll(path string) error

	Chown(path string, uid, gid int) error
	Chmod(path string, mode fs.FileMode) error
	Chtimes(path string, atime time.Time, mtime time.Time) error

	WalkDir(root string, fn WalkDirFunc) error
}

type WatchableFS interface {
	FS
	Watcher
}
