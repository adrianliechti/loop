package sshuttle

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"

	"github.com/Masterminds/semver"
)

var (
	minimalVersion = semver.MustParse("0.78.0")

	errNotFound = errors.New("sshuttle not found. see https://sshuttle.readthedocs.io/en/stable/installation.html")
	errOutdated = errors.New("sshuttle is outdated. see https://sshuttle.readthedocs.io/en/stable/installation.html")
)

func Info(ctx context.Context) (string, *semver.Version, error) {
	return path(ctx)
}

func path(ctx context.Context) (string, *semver.Version, error) {
	name := "sshuttle"

	if runtime.GOOS == "windows" {
		name = "sshuttle.exe"
		return "", nil, errors.New("windows currently not supported. try using WSL")
	}

	// verify global tool
	if path, err := exec.LookPath(name); err == nil {
		if version, err := version(ctx, path); err == nil {
			if !version.LessThan(minimalVersion) {
				return path, version, nil
			}

			return path, version, errOutdated
		}

		return path, nil, errOutdated
	}

	return "", nil, errNotFound
}

func version(ctx context.Context, path string) (*semver.Version, error) {
	cmd := exec.CommandContext(ctx, path, "-V")
	data, err := cmd.Output()

	if err != nil {
		return nil, err
	}

	version := strings.TrimSpace(string(data))
	return semver.NewVersion(version)
}
