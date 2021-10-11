package git

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/git"
)

var purgeCommand = &cli.Command{
	Name:  "purge",
	Usage: "delete (hidden) blobs in repository",

	Action: func(c *cli.Context) error {
		files := c.Args().Slice()
		return purge(c.Context, files)
	},
}

func purge(ctx context.Context, files []string) error {
	if len(files) == 0 {
		return nil
	}

	tool, _, err := git.Tool(ctx)

	if err != nil {
		return err
	}

	path, err := os.Getwd()

	if err != nil {
		return err
	}

	fetch := exec.CommandContext(ctx, tool, "fetch", "--all", "--prune", "--tags", "--prune-tags", "--force")
	fetch.Dir = path

	if err := fetch.Run(); err != nil {
		return err
	}

	branch := exec.CommandContext(ctx, tool, "branch", "-r")
	branch.Dir = path

	branchlist, err := branch.Output()

	if err != nil {
		return err
	}

	var branches []string

	branchscanner := bufio.NewScanner(bytes.NewReader(branchlist))

	for branchscanner.Scan() {
		line := strings.TrimSpace(branchscanner.Text())

		if !strings.HasPrefix(line, "origin/") {
			continue
		}

		if strings.Contains(line, "->") || strings.Contains(line, "HEAD") {
			continue
		}

		branch := strings.TrimPrefix(line, "origin/")
		branches = append(branches, branch)
	}

	if err := branchscanner.Err(); err != nil {
		return err
	}

	for _, branch := range branches {
		checkout := exec.CommandContext(ctx, tool, "branch", "--track", branch, "origin/"+branch)
		checkout.Dir = path
		checkout.Stdout = os.Stdout
		checkout.Stderr = os.Stderr

		if err := checkout.Run(); err != nil {
			//return err
		}
	}

	elems := make([]string, len(files))

	for _, file := range files {
		elems = append(elems, "'"+strings.TrimPrefix(file, "/")+"'")
	}

	filterbranch := exec.CommandContext(ctx, tool, "filter-branch", "--force", "--index-filter", "git rm -rf --cached --ignore-unmatch"+strings.Join(elems, " "), "--prune-empty", "--tag-name-filter", "cat", "--", "--all")
	filterbranch.Dir = path
	filterbranch.Stdout = os.Stdout
	filterbranch.Stderr = os.Stderr

	if err := filterbranch.Run(); err != nil {
		return err
	}

	if err := os.RemoveAll(filepath.Join(path, ".git/refs/original/")); err != nil {
		return err
	}

	reflog := exec.CommandContext(ctx, tool, "reflog", "expire", "--all")
	reflog.Dir = path
	reflog.Stdout = os.Stdout
	reflog.Stderr = os.Stderr

	if err := reflog.Run(); err != nil {
		return err
	}

	gc := exec.CommandContext(ctx, tool, "gc", "--aggressive", "--prune")
	gc.Dir = path
	gc.Stdout = os.Stdout
	gc.Stderr = os.Stderr

	if err := gc.Run(); err != nil {
		return err
	}

	pushall := exec.CommandContext(ctx, tool, "push", "origin", "--all", "--force")
	pushall.Dir = path
	pushall.Stdout = os.Stdout
	pushall.Stderr = os.Stderr

	if err := pushall.Run(); err != nil {
		return err
	}

	pushtags := exec.CommandContext(ctx, tool, "push", "origin", "--tags", "--force")
	pushtags.Dir = path
	pushtags.Stdout = os.Stdout
	pushtags.Stderr = os.Stderr

	if err := pushtags.Run(); err != nil {
		return err
	}

	return nil
}
