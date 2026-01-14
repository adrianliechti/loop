package docker

import (
	"strings"
)

type Identity struct {
	UID int
	GID int
}

type Port struct {
	Source int
	Target int
}

type Volume struct {
	Source string
	Target string

	Identity *Identity
}

type SyncMode string

const (
	SyncModeNone  SyncMode = ""
	SyncModeMount SyncMode = "mount"
)

const (
	resourceLabelSelector = "loop.cluster.local/docker"
)

func resourceName(name string) string {
	return "loop-docker-" + strings.ToLower(name)
}

func resourceLabels(name string) map[string]string {
	return map[string]string{
		resourceLabelSelector: strings.ToLower(name),
	}
}
