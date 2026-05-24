package docker

import "strings"

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
