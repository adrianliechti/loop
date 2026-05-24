package sftp

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
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

// handler routes every filesystem operation through *os.Root so that path
// traversal and symlink-based escapes outside the configured root are
// rejected by the kernel rather than relying on per-path validation.
type handler struct {
	root *os.Root
}

// toRelPath maps an SFTP-supplied (slash-separated) absolute-style path to a
// path relative to handler's root. *os.Root methods require relative paths;
// they also reject ".." components, but we Clean defensively in case clients
// send unnormalized paths.
func toRelPath(p string) string {
	p = path.Clean("/" + p)
	p = p[1:] // strip leading "/"

	if p == "" {
		return "."
	}

	return p
}

const (
	perm = 0o644

	methodList = "List"
	methodStat = "Stat"

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

	name := toRelPath(r.Filepath)

	switch r.Method {
	case methodList:
		dir, err := h.root.Open(name)

		if err != nil {
			return nil, err
		}

		defer dir.Close()

		entries, err := dir.ReadDir(-1)

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
		info, err := h.root.Stat(name)

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

	info, err := h.root.Lstat(toRelPath(r.Filepath))

	if err != nil {
		return nil, err
	}

	return listerat{info}, nil
}

// Methods: Readlink
func (h *handler) Readlink(p string) (string, error) {
	return h.root.Readlink(toRelPath(p))
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

	return h.root.OpenFile(toRelPath(r.Filepath), flag, perm)
}

// Methods: Setstat, Rmdir, Mkdir, Link, Symlink, Remove
func (h *handler) Filecmd(r *sftp.Request) error {
	if r.Filepath == "" {
		return os.ErrInvalid
	}

	name := toRelPath(r.Filepath)

	switch r.Method {
	case methodSetStat:
		attrs := r.Attributes()
		attrFlags := r.AttrFlags()

		if attrFlags.Acmodtime {
			atime := time.Unix(int64(attrs.Atime), 0)
			mtime := time.Unix(int64(attrs.Mtime), 0)

			if err := h.root.Chtimes(name, atime, mtime); err != nil {
				return err
			}
		}

		if attrFlags.Permissions {
			if err := h.root.Chmod(name, attrs.FileMode()); err != nil {
				return err
			}
		}

		if attrFlags.UidGid {
			if err := h.root.Chown(name, int(attrs.UID), int(attrs.GID)); err != nil {
				return err
			}
		}

		if attrFlags.Size {
			if err := h.truncate(name, int64(attrs.Size)); err != nil {
				return err
			}
		}

		return nil

	case methodRename:
		return h.PosixRename(r)

	case methodRmdir:
		info, err := h.root.Lstat(name)

		if err != nil {
			return err
		}

		if !info.IsDir() {
			return fmt.Errorf("%q is not a directory", r.Filepath)
		}

		// SFTP rmdir semantics: only remove an empty directory. Recursing
		// here (RemoveAll) would let a remote unlink wipe an entire local
		// subtree in mount/sync mode.
		return h.root.Remove(name)

	case methodMkdir:
		return h.root.MkdirAll(name, perm)

	case methodLink:
		if r.Target == "" {
			return os.ErrInvalid
		}

		return h.root.Link(name, toRelPath(r.Target))

	case methodSymlink:
		if r.Target == "" {
			return os.ErrInvalid
		}

		// Per pkg/sftp convention: r.Filepath is the symlink *target* (the
		// arbitrary string stored in the link), r.Target is the *linkpath*
		// (where the symlink is created). The target is kept as-is — *os.Root
		// rejects any traversal when the link is later resolved.
		return h.root.Symlink(r.Filepath, toRelPath(r.Target))

	case methodRemove:
		info, err := h.root.Lstat(name)

		if err != nil {
			return err
		}

		if info.IsDir() {
			return fmt.Errorf("%q is a directory", r.Filepath)
		}

		return h.root.Remove(name)

	default:
		return sftp.ErrSSHFxOpUnsupported
	}
}

// Methods: Rename
func (h *handler) PosixRename(r *sftp.Request) error {
	if r.Filepath == "" || r.Target == "" {
		return os.ErrInvalid
	}

	return h.root.Rename(toRelPath(r.Filepath), toRelPath(r.Target))
}

// truncate sets a file's size via Open+Truncate since *os.Root has no direct
// Truncate method.
func (h *handler) truncate(name string, size int64) error {
	f, err := h.root.OpenFile(name, os.O_WRONLY, 0)

	if err != nil {
		return err
	}

	defer f.Close()

	return f.Truncate(size)
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
