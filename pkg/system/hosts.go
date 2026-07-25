package system

import (
	"fmt"
	"maps"
	"os"
	"runtime"
	"slices"
	"strings"

	"github.com/rogpeppe/go-internal/lockedfile"
)

type HostsSection struct {
	name string
	path string

	hosts map[string][]string
}

func NewHostsSection(name string) (*HostsSection, error) {
	path := "/etc/hosts"

	if runtime.GOOS == "windows" {
		path = os.ExpandEnv("${SystemRoot}/System32/drivers/etc/hosts")
	}

	if val, ok := os.LookupEnv("HOSTS_PATH"); ok {
		path = val
	}

	return &HostsSection{
		name: name,
		path: path,

		hosts: make(map[string][]string),
	}, nil
}

// Add replaces the entries for address. The hosts are cloned: callers pass a
// slice they keep using (a tunnel's own host list), and Flush must not be able
// to reorder it underneath them.
func (s *HostsSection) Add(address string, hosts ...string) {
	s.hosts[address] = slices.Clone(hosts)
}

func (s *HostsSection) Remove(address string) {
	delete(s.hosts, address)
}

func (s *HostsSection) Clear() {
	clear(s.hosts)
}

// Flush rewrites this section in the hosts file, leaving the rest of it alone.
//
// The read and the write happen under one lock. The catapult and the gateway
// each own a section of the same file and flush concurrently on their own
// refresh timers, so reading and writing under separate locks lets the later
// writer rebuild the file from a snapshot taken before the earlier one landed —
// reverting the other section, and on shutdown leaving entries behind that
// point at loopback aliases that no longer exist.
func (s *HostsSection) Flush() error {
	return lockedfile.Transform(s.path, func(data []byte) ([]byte, error) {
		return []byte(s.render(string(data))), nil
	})
}

// render returns text with this section replaced by the current entries.
func (s *HostsSection) render(text string) string {
	ln := "\n"

	if runtime.GOOS == "windows" {
		ln = "\r\n"
	}

	headerStart := fmt.Sprintf("# Start Section %s%s", s.name, ln)
	headerEnd := fmt.Sprintf("# End Section %s%s", s.name, ln)

	sectionStart := strings.Index(text, headerStart)
	sectionEnd := strings.LastIndex(text, headerEnd)

	if sectionStart >= 0 && sectionEnd >= 0 && sectionEnd > sectionStart {
		text = text[:sectionStart] + text[sectionEnd+len(headerEnd):]
	}

	if len(s.hosts) > 0 {
		text = strings.TrimRight(text, ln) + ln + ln
		text += headerStart

		// Sorted so an unchanged section renders byte-identical. This runs on
		// every refresh, and map order would otherwise reshuffle every line of
		// the section every ten seconds.
		for _, address := range slices.Sorted(maps.Keys(s.hosts)) {
			for _, host := range slices.Sorted(slices.Values(s.hosts[address])) {
				text += fmt.Sprintf("%s %s%s", address, host, ln)
			}
		}

		text += fmt.Sprintf("%s%s", headerEnd, ln)
	}

	return strings.TrimRight(text, ln) + ln
}
