package sftp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
)

type Server struct {
	server *ssh.Server
}

func NewServer(addr, path string) *Server {
	s := &ssh.Server{
		Addr: addr,

		Handler: func(s ssh.Session) {
			io.WriteString(s, "SFTP server ready. Use SFTP for file transfer.\n")
		},

		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": func(s ssh.Session) {

				srv := NewRequestServer(s, path)

				if err := srv.Serve(); err != nil {
					srv.Close()
				}
			},
		},
	}

	return &Server{
		server: s,
	}
}

func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		if errors.Is(err, ssh.ErrServerClosed) {
			return nil
		}

		return err
	}

	if err := s.server.Close(); err != nil {
		if errors.Is(err, ssh.ErrServerClosed) {
			return nil
		}

		return err
	}

	return nil
}

func (s *Server) ListenAndServe() error {
	if err := s.server.ListenAndServe(); err != nil {
		if errors.Is(err, ssh.ErrServerClosed) {
			return nil
		}

		return err
	}

	return nil
}

type RequestServer struct {
	root string

	server *sftp.RequestServer
}

func NewRequestServer(session io.ReadWriteCloser, root string) *RequestServer {
	handler := &handler{root}

	s := sftp.NewRequestServer(session, sftp.Handlers{
		FileGet:  handler,
		FilePut:  handler,
		FileCmd:  handler,
		FileList: handler,
	})

	return &RequestServer{
		root: root,

		server: s,
	}
}

func (s *RequestServer) Serve() error {
	return s.server.Serve()
}

func (s *RequestServer) Close() error {
	return s.server.Close()
}

const (
	perm = 0o644

	methodGet      = "Get"
	methodPut      = "Put"
	methodOpen     = "Open"
	methodSetStat  = "Setstat"
	methodRename   = "Rename"
	methodRmdir    = "Rmdir"
	methodMkdir    = "Mkdir"
	methodLink     = "Link"
	methodSymlink  = "Symlink"
	methodRemove   = "Remove"
	methodList     = "List"
	methodStat     = "Stat"
	methodLstat    = "Lstat"
	methodReadlink = "Readlink"
)

type handler struct {
	root string
}

func (h *handler) openFile(req *sftp.Request) (*os.File, error) {
	path := filepath.Join(h.root, req.Filepath)

	var flags int

	pflags := req.Pflags()

	if pflags.Append {
		flags |= os.O_APPEND
	}

	if pflags.Creat {
		flags |= os.O_CREATE
	}

	if pflags.Excl {
		flags |= os.O_EXCL
	}

	if pflags.Trunc {
		flags |= os.O_TRUNC
	}

	if pflags.Read && pflags.Write {
		flags |= os.O_RDWR
	} else if pflags.Read {
		flags |= os.O_RDONLY
	} else if pflags.Write {
		flags |= os.O_WRONLY
	}

	return os.OpenFile(path, flags, perm)
}

func (h *handler) setStat(r *sftp.Request) error {
	path := filepath.Join(h.root, r.Filepath)

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
}

func (h *handler) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	if r.Filepath == "" {
		return nil, os.ErrInvalid
	}

	if !r.Pflags().Read {
		return nil, os.ErrInvalid
	}

	return h.openFile(r)
}

func (h *handler) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	if r.Filepath == "" {
		return nil, os.ErrInvalid
	}

	if !r.Pflags().Write {
		return nil, os.ErrInvalid
	}

	return h.openFile(r)
}

func (h *handler) Filecmd(r *sftp.Request) error {
	if r.Filepath == "" {
		return os.ErrInvalid
	}

	switch r.Method {
	case methodSetStat:
		return h.setStat(r)

	case methodRename:
		if r.Target == "" {
			return os.ErrInvalid
		}

		oldpath := filepath.Join(h.root, r.Filepath)
		newpath := filepath.Join(h.root, r.Target)

		return os.Rename(oldpath, newpath)

	case methodRmdir:
		path := filepath.Join(h.root, r.Filepath)

		info, err := os.Lstat(path)

		if err != nil {
			return err
		}

		if !info.IsDir() {
			return fmt.Errorf("%q is not a directory", r.Filepath)
		}

		return os.RemoveAll(path)

	case methodMkdir:
		path := filepath.Join(h.root, r.Filepath)

		return os.MkdirAll(path, perm)

	case methodLink:
		if r.Target == "" {
			return os.ErrInvalid
		}

		oldname := filepath.Join(h.root, r.Filepath)
		newname := filepath.Join(h.root, r.Target)

		return os.Link(oldname, newname)

	case methodSymlink:
		if r.Target == "" {
			return os.ErrInvalid
		}

		oldname := filepath.Join(h.root, r.Filepath)
		newname := filepath.Join(h.root, r.Target)

		return os.Symlink(oldname, newname)

	case methodRemove:
		path := filepath.Join(h.root, r.Filepath)

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

func (h *handler) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	if r.Filepath == "" {
		return nil, os.ErrInvalid
	}

	path := filepath.Join(h.root, r.Filepath)

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

		return listerAt(infos), nil

	case methodStat:
		info, err := os.Stat(path)

		if err != nil {
			return nil, err
		}

		return listerAt{info}, nil

	case methodReadlink:
		dst, err := os.Readlink(path)

		if err != nil {
			return nil, err
		}

		return listerAt{fileName(dst)}, nil

	default:
		return nil, sftp.ErrSSHFxOpUnsupported
	}
}

type listerAt []fs.FileInfo

func (l listerAt) ListAt(ls []fs.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(l)) {
		return 0, io.EOF
	}

	n := copy(ls, l[offset:])

	if n < len(ls) {
		return n, io.EOF
	}

	return n, nil
}

type fileName string

func (f fileName) Name() string {
	return string(f)
}

func (f fileName) Size() int64 {
	return 0
}

func (f fileName) Mode() fs.FileMode {
	return 0
}

func (f fileName) ModTime() time.Time {
	return time.Time{}
}

func (f fileName) IsDir() bool {
	return false
}

func (f fileName) Sys() any {
	return nil
}
