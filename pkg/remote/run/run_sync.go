package run

// import (
// 	"context"
// 	"io"
// 	"os"
// 	"path"
// 	"path/filepath"
// 	"strings"

// 	"github.com/adrianliechti/loop/pkg/cli"
// 	"github.com/adrianliechti/loop/pkg/fs"
// 	"github.com/google/uuid"
// )

// func syncLocalChanges(ctx context.Context, local, remote fs.WatchableFS, volumes []Volume) error {
// 	root := "/data"

// 	var paths []string

// 	for _, v := range volumes {
// 		paths = append(paths, v.Source)
// 	}

// 	events, err := local.Watch(ctx, paths...)

// 	if err != nil {
// 		return err
// 	}

// 	for e := range events {
// 		if ignoredChange(e.Path) {
// 			continue
// 		}

// 		remotePath := path.Join(root, mapRemotePath(volumes, e.Path))

// 		if remotePath == "" {
// 			continue
// 		}

// 		// println(e.Action, e.Path, remotePath)

// 		switch e.Action {
// 		case fs.Create, fs.Modify:
// 			info, err := local.Stat(e.Path)

// 			if err != nil {
// 				continue
// 			}

// 			if r := path.Join(root, mapRemotePath(volumes, e.RenamedFrom)); r != root {
// 				if _, err := remote.Stat(r); err == nil {
// 					if err := remote.Rename(r, remotePath); err != nil {
// 						cli.Error("rename remote dir", err)
// 						continue
// 					}

// 					continue
// 				}
// 			}

// 			if i, err := remote.Stat(remotePath); err == nil {
// 				if i.ModTime().After(info.ModTime()) || i.ModTime().Equal(info.ModTime()) {
// 					continue
// 				}
// 			}

// 			if info.IsDir() {
// 				if err := syncDir(ctx, remote, remotePath, local, e.Path); err != nil {
// 					cli.Error("create remote dir", err)
// 					continue
// 				}

// 				continue
// 			}

// 			if err := syncFile(ctx, remote, remotePath, local, e.Path); err != nil {
// 				cli.Error("create remote file", err)
// 				continue
// 			}

// 		case fs.Remove:
// 			info, err := remote.Stat(remotePath)

// 			if err != nil {
// 				continue
// 			}

// 			if info.IsDir() {
// 				if err := remote.RemoveAll(remotePath); err != nil {
// 					cli.Error("remove remote dir", err)
// 					continue
// 				}

// 				continue
// 			}

// 			if err := remote.Remove(remotePath); err != nil {
// 				cli.Error("remove remote file", err)
// 				continue
// 			}
// 		}
// 	}

// 	return nil
// }

// func syncRemoteVolumes(ctx context.Context, local, remote fs.WatchableFS, volumes []Volume) error {
// 	root := "/data"

// 	var paths []string

// 	for _, v := range volumes {
// 		paths = append(paths, path.Join("/data", v.Target))
// 	}

// 	events, err := remote.Watch(ctx, paths...)

// 	if err != nil {
// 		return err
// 	}

// 	for e := range events {
// 		if ignoredChange(e.Path) {
// 			continue
// 		}

// 		localPath := mapLocalPath(volumes, strings.TrimPrefix(e.Path, root))

// 		if localPath == "" {
// 			continue
// 		}

// 		// println(e.Action, e.Path, localPath)

// 		switch e.Action {
// 		case fs.Create, fs.Modify:
// 			info, err := remote.Stat(e.Path)

// 			if err != nil {
// 				continue
// 			}

// 			if l := mapLocalPath(volumes, strings.TrimPrefix(e.RenamedFrom, root)); l != "" {
// 				if _, err := local.Stat(l); err == nil {
// 					if err := local.Rename(l, localPath); err != nil {
// 						cli.Error("remove local file", err)
// 						continue
// 					}

// 					continue
// 				}
// 			}

// 			if i, err := local.Stat(localPath); err == nil {
// 				if i.ModTime().After(info.ModTime()) || i.ModTime().Equal(info.ModTime()) {
// 					continue
// 				}
// 			}

// 			if info.IsDir() {
// 				if err := syncDir(ctx, local, localPath, remote, e.Path); err != nil {
// 					cli.Error("create local dir", err)
// 					continue
// 				}

// 				continue
// 			}

// 			if err := syncFile(ctx, local, localPath, remote, e.Path); err != nil {
// 				cli.Error("create local file", err)
// 				continue
// 			}

// 		case fs.Remove:
// 			info, err := local.Stat(localPath)

// 			if err != nil {
// 				continue
// 			}

// 			if info.IsDir() {
// 				if err := local.RemoveAll(localPath); err != nil {
// 					cli.Error("delete local dir", err)
// 					continue
// 				}

// 				continue
// 			}

// 			if err := local.Remove(localPath); err != nil {
// 				cli.Error("delete local file", err)
// 				continue
// 			}
// 		}
// 	}

// 	return nil
// }

// func ignoredChange(name string) bool {
// 	ext := path.Ext(name)

// 	if ext == ".tmp" {
// 		return true
// 	}

// 	return false
// }

// func mapLocalPath(volumes []Volume, remotePath string) string {
// 	if remotePath == "" {
// 		return ""
// 	}

// 	longestMatch := ""
// 	longestValue := ""

// 	for _, v := range volumes {
// 		if strings.HasPrefix(remotePath, v.Target) && len(v.Target) > len(longestMatch) {
// 			longestMatch = v.Target
// 			longestValue = v.Source
// 		}
// 	}

// 	rel, _ := filepath.Rel(longestMatch, filepath.FromSlash(remotePath))
// 	return filepath.Join(longestValue, rel)
// }

// func mapRemotePath(volumes []Volume, localPath string) string {
// 	if localPath == "" {
// 		return ""
// 	}

// 	longestMatch := ""
// 	longestValue := ""

// 	for _, v := range volumes {
// 		if strings.HasPrefix(localPath, v.Source) && len(v.Source) > len(longestMatch) {
// 			longestMatch = v.Source
// 			longestValue = v.Target
// 		}
// 	}

// 	rel := relPath(longestMatch, filepath.ToSlash(localPath))
// 	return path.Join(longestValue, rel)
// }

// func syncFile(ctx context.Context, src fs.FS, srcPath string, dst fs.FS, dstPath string) error {
// 	tmp := path.Join(path.Dir(srcPath), uuid.NewString()+".tmp")

// 	t, err := src.Create(tmp)

// 	if err != nil {
// 		return err
// 	}

// 	defer src.Remove(tmp)

// 	defer t.Close()

// 	f, err := dst.Open(dstPath)

// 	if err != nil {
// 		return err
// 	}

// 	i, err := f.Stat()

// 	if err != nil {
// 		return err
// 	}

// 	defer f.Close()

// 	if _, err := io.Copy(t, f); err != nil {
// 		return err
// 	}

// 	t.Close()
// 	f.Close()

// 	os.Chtimes(t.Name(), i.ModTime(), i.ModTime())

// 	dir := path.Dir(t.Name())
// 	src.MkdirAll(dir, 0755)

// 	return src.Rename(t.Name(), srcPath)
// }

// func syncDir(ctx context.Context, src fs.FS, srcPath string, dst fs.FS, dstPath string) error {
// 	return dst.WalkDir(dstPath, func(dirPath string, d fs.DirEntry, err error) error {
// 		if err != nil {
// 			return err
// 		}

// 		rel := relPath(dstPath, dirPath)
// 		name := path.Join(srcPath, rel)

// 		i, err := d.Info()

// 		if err != nil {
// 			return err
// 		}

// 		if d.IsDir() {
// 			if err := src.MkdirAll(name, i.Mode()); err != nil {
// 				return err
// 			}

// 			src.Chtimes(name, i.ModTime(), i.ModTime())

// 			return nil
// 		}

// 		return syncFile(ctx, src, name, dst, dirPath)
// 	})
// }

// func relPath(base, targpath string) string {
// 	base = path.Clean(filepath.ToSlash(base))

// 	targpath = path.Clean(filepath.ToSlash(targpath))
// 	targpath = strings.TrimLeft(targpath, "/")

// 	rel := strings.TrimPrefix(targpath, base)
// 	rel = strings.TrimLeft(rel, "/")

// 	return rel
// }
