package ssh

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/Masterminds/semver"
)

var (
	minimalVersion = semver.MustParse("7.0.0")

	errNotFound = errors.New("ssh not found. please install")
	errOutdated = errors.New("ssh is outdated. please upgrade")
)

func Info(ctx context.Context) (string, *semver.Version, error) {
	return path(ctx)
}

func path(ctx context.Context) (string, *semver.Version, error) {
	name := "ssh"

	if runtime.GOOS == "windows" {
		name = "ssh.exe"
	}

	// verify global tool
	if path, err := exec.LookPath(name); err == nil {
		if version, err := version(ctx, path); err == nil {
			if !version.LessThan(minimalVersion) {
				return path, version, nil
			}
		}

		return path, nil, errOutdated
	}

	return "", nil, errNotFound
}

func version(ctx context.Context, path string) (*semver.Version, error) {
	cmd := exec.CommandContext(ctx, path, "-V")
	data, err := cmd.CombinedOutput()

	if err != nil {
		return nil, err
	}

	version := strings.TrimSpace(string(data))

	r, _ := regexp.Compile(`OpenSSH_(for_Windows_)?(\d+\.\d+(\.\d+)?).*`)

	if matches := r.FindStringSubmatch(version); len(matches) == 4 {
		return semver.NewVersion(matches[2])
	}

	return semver.NewVersion("0.0.0")
}
