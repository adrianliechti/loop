package git

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/Masterminds/semver"
)

var (
	minimalVersion = semver.MustParse("2.0.0")

	errNotFound = errors.New("git not found. see https://git-scm.com/download")
	errOutdated = errors.New("git is outdated. see https://git-scm.com/download")
)

func Tool(ctx context.Context) (string, *semver.Version, error) {
	if path, version, err := Path(ctx); err == nil {
		return path, version, err
	}

	return "", nil, errNotFound
}

func Path(ctx context.Context) (string, *semver.Version, error) {
	name := "git"

	if runtime.GOOS == "windows" {
		name = "git.exe"
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
	cmd := exec.CommandContext(ctx, path, "--version")
	data, err := cmd.Output()

	if err != nil {
		return nil, err
	}

	version := strings.TrimSpace(string(data))

	r, _ := regexp.Compile(`^.* version (\d+\.\d+(\.\d+)?).*`)

	if matches := r.FindStringSubmatch(version); len(matches) == 3 {
		return semver.NewVersion(matches[1])
	}

	return semver.NewVersion("0.0.0")
}

func Clone(ctx context.Context, path, uri, username, password string) error {
	tool, _, err := Tool(ctx)

	if err != nil {
		return err
	}

	u, err := url.Parse(uri)

	if err != nil {
		return err
	}

	if username != "" && password != "" {
		u.User = url.UserPassword(username, password)
	}

	cmd := exec.CommandContext(ctx, tool, "clone", u.String(), path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
