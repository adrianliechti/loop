package git

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/adrianliechti/loop/pkg/cli"
	"github.com/adrianliechti/loop/pkg/git"
)

var blobsCommand = &cli.Command{
	Name:  "blobs",
	Usage: "list (hidden) blobs in repository",

	Action: func(c *cli.Context) error {

		return blobs(c.Context)
	},
}

func blobs(ctx context.Context) error {
	tool, _, err := git.Tool(ctx)

	if err != nil {
		return err
	}

	path, err := os.Getwd()

	if err != nil {
		return err
	}

	r, w := io.Pipe()
	var output bytes.Buffer

	reflist := exec.CommandContext(ctx, tool, "rev-list", "--objects", "--all")
	reflist.Stdout = w
	reflist.Dir = path

	catfile := exec.CommandContext(ctx, tool, "cat-file", "--batch-check=%(objecttype) %(objectname) %(objectsize) %(rest)")
	catfile.Stdin = r
	catfile.Stdout = &output
	catfile.Dir = path

	if err != nil {
		return err
	}

	if err := reflist.Start(); err != nil {
		return err
	}

	if err := catfile.Start(); err != nil {
		return err
	}

	if err := reflist.Wait(); err != nil {
		return err
	}

	w.Close()

	if err := catfile.Wait(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(&output)

	var items [][]string

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "blob ") {
			continue
		}

		parts := strings.SplitN(line, " ", 4)
		items = append(items, parts[1:])
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	sort.SliceStable(items, func(i, j int) bool {
		sizeI, _ := strconv.ParseInt(items[i][1], 10, 64)
		sizeJ, _ := strconv.ParseInt(items[j][1], 10, 64)
		return sizeI < sizeJ
	})

	cli.Table([]string{"Commit", "Size", "Path"}, items)

	return nil
}
