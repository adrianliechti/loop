package kubectl

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"runtime"

	"github.com/Masterminds/semver"
)

var (
	minimalVersion = semver.MustParse("1.19.0")

	errNotFound = errors.New("kubectl not found. see https://kubernetes.io/docs/tasks/tools/install-kubectl")
	errOutdated = errors.New("kubectl is outdated. see https://kubernetes.io/docs/tasks/tools/install-kubectl")
)

func Tool(ctx context.Context) (string, *semver.Version, error) {
	if path, version, err := Path(ctx); err == nil {
		return path, version, err
	}

	return "", nil, errNotFound
}

func Path(ctx context.Context) (string, *semver.Version, error) {
	name := "kubectl"

	if runtime.GOOS == "windows" {
		name = "kubectl.exe"
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
	type versionType struct {
		ClientVersion struct {
			BuildDate    string `json:"buildDate"`
			Compiler     string `json:"compiler"`
			GitCommit    string `json:"gitCommit"`
			GitTreeState string `json:"gitTreeState"`
			GitVersion   string `json:"gitVersion"`
			GoVersion    string `json:"goVersion"`
			Major        string `json:"major"`
			Minor        string `json:"minor"`
			Platform     string `json:"platform"`
		} `json:"clientVersion"`
	}

	cmd := exec.CommandContext(ctx, path, "version", "--client", "-o", "json")
	data, err := cmd.Output()

	if err != nil {
		return nil, err
	}

	var version versionType

	if err := json.Unmarshal(data, &version); err != nil {
		return nil, err
	}

	return semver.NewVersion(version.ClientVersion.GitVersion)
}

func Exec(ctx context.Context, kubeconfig, namespace, name, container string, path string, arg ...string) error {
	tool, _, err := Tool(ctx)

	if err != nil {
		return err
	}

	args := []string{
		"--kubeconfig",
		kubeconfig,
		"exec",
		"-it",
		"-n",
		namespace,
		name,
	}

	if container != "" {
		args = append(args, "-c", container)
	}

	args = append(args, "--")
	args = append(args, path)
	args = append(args, arg...)

	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func Attach(ctx context.Context, kubeconfig, namespace, name, container string) error {
	tool, _, err := Tool(ctx)

	if err != nil {
		return err
	}

	args := []string{
		"--kubeconfig",
		kubeconfig,
		"attach",
		"-it",
		"-n",
		namespace,
		name,
	}

	if container != "" {
		args = append(args, "-c", container)
	}

	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
