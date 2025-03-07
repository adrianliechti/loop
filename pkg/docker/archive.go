package docker

import (
	"archive/tar"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

type TarballOptions struct {
	UID *int
	GID *int
}

func WriteTarball(w io.Writer, root string, options *TarballOptions) error {
	if options == nil {
		options = new(TarballOptions)
	}

	t := tar.NewWriter(w)
	defer t.Close()

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)

		if err != nil {
			return err
		}

		if rel == "." {
			return nil
		}

		rel = filepath.ToSlash(rel)

		info, err := os.Lstat(path)

		if err != nil {
			return err
		}

		isDir := info.IsDir()
		isLink := info.Mode()&os.ModeSymlink != 0

		link := info.Name()

		if isLink {
			target, err := os.Readlink(path)

			if err != nil {
				return err
			}

			link = filepath.ToSlash(target)
		}

		hdr, err := tar.FileInfoHeader(info, link)

		if err != nil {
			return err
		}

		hdr.Name = rel

		if runtime.GOOS == "windows" {
			if isDir {
				hdr.Mode = 0755
			} else {
				hdr.Mode = 0644
			}
		}

		if options.UID != nil {
			hdr.Uid = *options.UID
		}

		if options.GID != nil {
			hdr.Gid = *options.GID
		}

		if err := t.WriteHeader(hdr); err != nil {
			return err
		}

		if isDir || isLink {
			return nil
		}

		f, err := os.Open(path)

		if err != nil {
			return err
		}

		defer f.Close()

		if _, err := io.Copy(t, f); err != nil {
			return err
		}

		return f.Close()
	})
}
