package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func Abs(path string) (string, error) {
	return filepath.Abs(path)
}

func EmptyDir(path, name string) (string, error) {
	if path == "" {
		wd, err := os.Getwd()

		if err != nil {
			return "", err
		}

		path = wd
	}

	if ok, err := IsEmptyDir(path); err == nil && ok {
		return path, nil
	}

	path = filepath.Join(path, name)

	if err := os.MkdirAll(path, 0755); err != nil {
		return "", err
	}

	if ok, err := IsEmptyDir(path); err == nil && ok {
		return path, nil
	}

	return "", errors.New("directory already exists or is not empty")
}

func IsEmptyDir(path string) (bool, error) {
	info, err := os.Stat(path)

	if err != nil {
		return false, err
	}

	if !info.IsDir() {
		return false, errors.New("path is not a directory")
	}

	path = strings.TrimRight(path, "/") + "/"

	walkerr := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if p == path {
			return nil
		}

		if strings.EqualFold(info.Name(), ".git") {
			return nil
		}

		if strings.EqualFold(info.Name(), ".DS_Store") {
			return nil
		}

		return errors.New("directory is not empty")
	})

	if walkerr != nil {
		return false, err
	}

	return true, nil
}
