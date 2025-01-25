package sftp

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/sftp"
)

var (
	_ sftp.FileLister         = (*handler)(nil)
	_ sftp.LstatFileLister    = (*handler)(nil)
	_ sftp.ReadlinkFileLister = (*handler)(nil)

	_ sftp.FileReader     = (*handler)(nil)
	_ sftp.FileWriter     = (*handler)(nil)
	_ sftp.OpenFileWriter = (*handler)(nil)

	_ sftp.FileCmder            = (*handler)(nil)
	_ sftp.PosixRenameFileCmder = (*handler)(nil)
)

type handler struct {
	root string
}

func (h *handler) toLocalPath(path string) string {
	path = filepath.FromSlash(path)
	path = filepath.Clean(path)

	return filepath.Join(h.root, path)
}

const (
	perm = 0o644

	methodList = "List"
	methodStat = "Stat"
	// methodLstat   = "Lstat"

	// methodGet     = "Get"
	// methodPut     = "Put"
	// methodOpen    = "Open"

	methodSetStat = "Setstat"
	methodRename  = "Rename"
	methodRmdir   = "Rmdir"
	methodMkdir   = "Mkdir"
	methodLink    = "Link"
	methodSymlink = "Symlink"
	methodRemove  = "Remove"
)

// Methods: List, Stat
func (h *handler) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	if r.Filepath == "" {
		return nil, os.ErrInvalid
	}

	path := h.toLocalPath(r.Filepath)

	switch r.Method {
	case methodList:
		entries, err := os.ReadDir(path)

		if err != nil {
			return nil, err
		}

		infos := make([]fs.FileInfo, len(entries))

		for i, entry := range entries {
			info, err := entry.Info()

			if err != nil {
				return nil, err
			}

			infos[i] = info
		}

		return listerat(infos), nil

	case methodStat:
		info, err := os.Stat(path)

		if err != nil {
			return nil, err
		}

		return listerat{info}, nil

	default:
		return nil, sftp.ErrSSHFxOpUnsupported
	}
}

// Methods: Lstat
func (h *handler) Lstat(r *sftp.Request) (sftp.ListerAt, error) {
	if r.Filepath == "" {
		return nil, os.ErrInvalid
	}

	path := h.toLocalPath(r.Filepath)

	info, err := os.Lstat(path)

	if err != nil {
		return nil, err
	}

	return listerat{info}, nil
}

// Methods: Readlink
func (h *handler) Readlink(path string) (string, error) {
	path = h.toLocalPath(path)

	return os.Readlink(path)
}

// Methods: Get
func (h *handler) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	return h.OpenFile(r)
}

// Methods: Put
func (h *handler) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	return h.OpenFile(r)
}

// Methods: Open
func (h *handler) OpenFile(r *sftp.Request) (sftp.WriterAtReaderAt, error) {
	if r.Filepath == "" {
		return nil, os.ErrInvalid
	}

	path := h.toLocalPath(r.Filepath)

	var flag int

	pflags := r.Pflags()

	if pflags.Append {
		flag |= os.O_APPEND
	}

	if pflags.Creat {
		flag |= os.O_CREATE
	}

	if pflags.Excl {
		flag |= os.O_EXCL
	}

	if pflags.Trunc {
		flag |= os.O_TRUNC
	}

	if pflags.Read && pflags.Write {
		flag |= os.O_RDWR
	} else if pflags.Read {
		flag |= os.O_RDONLY
	} else if pflags.Write {
		flag |= os.O_WRONLY
	}

	return os.OpenFile(path, flag, perm)
}

// Methods: Setstat, Rmdir, Mkdir, Link, Symlink, Remove
func (h *handler) Filecmd(r *sftp.Request) error {
	if r.Filepath == "" {
		return os.ErrInvalid
	}

	switch r.Method {
	case methodSetStat:
		path := h.toLocalPath(r.Filepath)

		attrs := r.Attributes()
		attrFlags := r.AttrFlags()

		if attrFlags.Acmodtime {
			atime := time.Unix(int64(attrs.Atime), 0)
			mtime := time.Unix(int64(attrs.Mtime), 0)

			err := os.Chtimes(path, atime, mtime)
			if err != nil {
				return err
			}
		}

		if attrFlags.Permissions {
			err := os.Chmod(path, attrs.FileMode())
			if err != nil {
				return err
			}
		}

		if attrFlags.UidGid {
			if err := os.Chown(path, int(attrs.UID), int(attrs.GID)); err != nil {
				return err
			}
		}

		if attrFlags.Size {
			if err := os.Truncate(path, int64(attrs.Size)); err != nil {
				return err
			}
		}

		return nil

	case methodRename:
		return h.PosixRename(r)

	case methodRmdir:
		path := h.toLocalPath(r.Filepath)

		info, err := os.Lstat(path)

		if err != nil {
			return err
		}

		if !info.IsDir() {
			return fmt.Errorf("%q is not a directory", r.Filepath)
		}

		return os.RemoveAll(path)

	case methodMkdir:
		path := h.toLocalPath(r.Filepath)

		return os.MkdirAll(path, perm)

	case methodLink:
		if r.Target == "" {
			return os.ErrInvalid
		}

		oldname := h.toLocalPath(r.Filepath)
		newname := h.toLocalPath(r.Target)

		return os.Link(oldname, newname)

	case methodSymlink:
		if r.Target == "" {
			return os.ErrInvalid
		}

		oldname := h.toLocalPath(r.Filepath)
		newname := h.toLocalPath(r.Target)

		return os.Symlink(oldname, newname)

	case methodRemove:
		path := h.toLocalPath(r.Filepath)

		info, err := os.Lstat(path)

		if err != nil {
			return err
		}

		if info.IsDir() {
			return fmt.Errorf("%q is a directory", r.Filepath)
		}

		return os.Remove(path)

	default:
		return sftp.ErrSSHFxOpUnsupported
	}
}

// Methods: Rename
func (h *handler) PosixRename(r *sftp.Request) error {
	if r.Filepath == "" {
		return os.ErrInvalid
	}

	if r.Target == "" {
		return os.ErrInvalid
	}

	oldname := h.toLocalPath(r.Filepath)
	newname := h.toLocalPath(r.Target)

	return os.Rename(oldname, newname)
}

type listerat []os.FileInfo

func (f listerat) ListAt(ls []os.FileInfo, offset int64) (int, error) {
	var n int

	if offset >= int64(len(f)) {
		return 0, io.EOF
	}

	n = copy(ls, f[offset:])

	if n < len(ls) {
		return n, io.EOF
	}

	return n, nil
}
