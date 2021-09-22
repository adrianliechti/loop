package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/Masterminds/semver"
)

var (
	minimalVersion = semver.MustParse("19.0.0")

	errNotFound = errors.New("docker not found. see https://docs.docker.com/get-docker/")
	errOutdated = errors.New("docker is outdated. see https://docs.docker.com/get-docker/")
)

func Tool(ctx context.Context) (string, *semver.Version, error) {
	path, version, err := Path(ctx)

	if err != nil {
		return path, version, err
	}

	cmd := exec.CommandContext(ctx, path, "info")

	if err := cmd.Run(); err == nil {
		return path, version, nil
	}

	return path, version, errors.New("Docker Daemon seems not to be running")
}

func Path(ctx context.Context) (string, *semver.Version, error) {
	name := "docker"

	if runtime.GOOS == "windows" {
		name = "docker.exe"
	}

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
	output, _ := exec.CommandContext(ctx, path, "version", "--format", "{{.Client.Version}}").Output()

	parts := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")

	if len(parts) < 1 {
		return nil, errors.New("unable to get docker version")
	}

	version := strings.TrimSpace(parts[0])
	return semver.NewVersion(version)
}

func Login(ctx context.Context, address, username, password string) error {
	tool, _, err := Tool(ctx)

	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, tool, "login", "--username", username, "--password-stdin", address)
	cmd.Stdin = bytes.NewReader([]byte(password))

	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.New(strings.TrimSpace(string(output)))
	}

	return nil
}

func Pull(ctx context.Context, image string) error {
	tool, _, err := Tool(ctx)

	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, tool, "pull", image)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func Push(ctx context.Context, image string) error {
	tool, _, err := Tool(ctx)

	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, tool, "push", image)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func Tag(ctx context.Context, source, target string) error {
	tool, _, err := Tool(ctx)

	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, tool, "tag", source, target)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

type RunOptions struct {
	Dir string

	User string

	Tini       bool
	Privileged bool

	Env     map[string]string
	Ports   map[int]int
	Volumes map[string]string
}

func RunInteractive(ctx context.Context, image string, options RunOptions, args ...string) error {
	tool, _, err := Tool(ctx)

	if err != nil {
		return err
	}

	pull := exec.CommandContext(ctx, tool, "pull", "--quiet", image)

	if err := pull.Run(); err != nil {
		return err
	}

	runArgs := []string{
		"run", "-it", "--rm",
	}

	if options.Tini {
		runArgs = append(runArgs, "--init")
	}

	if options.Privileged {
		runArgs = append(runArgs, "--privileged")
	}

	if options.Dir != "" {
		runArgs = append(runArgs, "-w", options.Dir)
	}

	if options.User != "" {
		runArgs = append(runArgs, "-u", options.User)
	}

	for source, target := range options.Ports {
		runArgs = append(runArgs, "-p", fmt.Sprintf("127.0.0.1:%d:%d", source, target))
	}

	for source, target := range options.Volumes {
		runArgs = append(runArgs, "-v", source+":"+target)
	}

	for key, value := range options.Env {
		runArgs = append(runArgs, "-e", key+"="+value)
	}

	runArgs = append(runArgs, image)
	runArgs = append(runArgs, args...)

	run := exec.CommandContext(ctx, tool, runArgs...)
	run.Stdin = os.Stdin
	run.Stdout = os.Stdout
	run.Stderr = os.Stderr

	if err := run.Run(); err != nil {
		return err
	}

	return nil
}
