package sftp

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
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
// traversal and symlink-based escapes outside the configured root are rejected
// by the kernel rather than relying on per-path validation.
type handler struct {
	root   *os.Root
	mounts []rootMount
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

func (h *handler) resolve(p string) (*os.Root, string) {
	name := toRelPath(p)

	for _, mount := range h.mounts {
		if name == mount.target {
			return mount.root, "."
		}

		if mount.target != "." && strings.HasPrefix(name, mount.target+"/") {
			return mount.root, strings.TrimPrefix(name, mount.target+"/")
		}

		if mount.target == "." {
			return mount.root, name
		}
	}

	return h.root, name
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

	root, name := h.resolve(r.Filepath)

	switch r.Method {
	case methodList:
		dir, err := root.Open(name)

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
		info, err := root.Stat(name)

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

	root, name := h.resolve(r.Filepath)
	info, err := root.Lstat(name)

	if err != nil {
		return nil, err
	}

	return listerat{info}, nil
}

// Methods: Readlink
func (h *handler) Readlink(p string) (string, error) {
	root, name := h.resolve(p)
	return root.Readlink(name)
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

	root, name := h.resolve(r.Filepath)
	return root.OpenFile(name, flag, perm)
}

// Methods: Setstat, Rmdir, Mkdir, Link, Symlink, Remove
func (h *handler) Filecmd(r *sftp.Request) error {
	if r.Filepath == "" {
		return os.ErrInvalid
	}

	root, name := h.resolve(r.Filepath)

	switch r.Method {
	case methodSetStat:
		attrs := r.Attributes()
		attrFlags := r.AttrFlags()

		if attrFlags.Acmodtime {
			atime := time.Unix(int64(attrs.Atime), 0)
			mtime := time.Unix(int64(attrs.Mtime), 0)

			if err := root.Chtimes(name, atime, mtime); err != nil {
				return err
			}
		}

		if attrFlags.Permissions {
			if err := root.Chmod(name, attrs.FileMode()); err != nil {
				return err
			}
		}

		if attrFlags.UidGid {
			if err := root.Chown(name, int(attrs.UID), int(attrs.GID)); err != nil {
				return err
			}
		}

		if attrFlags.Size {
			if err := h.truncate(root, name, int64(attrs.Size)); err != nil {
				return err
			}
		}

		return nil

	case methodRename:
		return h.PosixRename(r)

	case methodRmdir:
		info, err := root.Lstat(name)

		if err != nil {
			return err
		}

		if !info.IsDir() {
			return fmt.Errorf("%q is not a directory", r.Filepath)
		}

		// SFTP rmdir semantics: only remove an empty directory. Recursing here
		// would let a remote unlink wipe an entire local subtree in mount mode.
		return root.Remove(name)

	case methodMkdir:
		return root.MkdirAll(name, perm)

	case methodLink:
		if r.Target == "" {
			return os.ErrInvalid
		}

		targetRoot, targetName := h.resolve(r.Target)

		if targetRoot != root {
			return fmt.Errorf("hard links across mounts are not supported")
		}

		return root.Link(name, targetName)

	case methodSymlink:
		if r.Target == "" {
			return os.ErrInvalid
		}

		linkRoot, linkName := h.resolve(r.Target)

		// Per pkg/sftp convention: r.Filepath is the symlink target string,
		// r.Target is the linkpath. The target string is kept as-is; *os.Root
		// rejects traversal when the link is later resolved.
		return linkRoot.Symlink(r.Filepath, linkName)

	case methodRemove:
		info, err := root.Lstat(name)

		if err != nil {
			return err
		}

		if info.IsDir() {
			return fmt.Errorf("%q is a directory", r.Filepath)
		}

		return root.Remove(name)

	default:
		return sftp.ErrSSHFxOpUnsupported
	}
}

// Methods: Rename
func (h *handler) PosixRename(r *sftp.Request) error {
	if r.Filepath == "" || r.Target == "" {
		return os.ErrInvalid
	}

	root, name := h.resolve(r.Filepath)
	targetRoot, targetName := h.resolve(r.Target)

	if targetRoot != root {
		return fmt.Errorf("renames across mounts are not supported")
	}

	return root.Rename(name, targetName)
}

// truncate sets a file's size via Open+Truncate since *os.Root has no direct
// Truncate method.
func (h *handler) truncate(root *os.Root, name string, size int64) error {
	f, err := root.OpenFile(name, os.O_WRONLY, 0)

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
