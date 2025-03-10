package system

import (
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
)

type HostsSection struct {
	name  string
	hosts map[string][]string
	file  *AtomicFile
}

func HostsFilePath() string {
	path := "/etc/hosts"

	if runtime.GOOS == "windows" {
		path = os.ExpandEnv("${SystemRoot}/System32/drivers/etc/hosts")
	}

	if val, ok := os.LookupEnv("HOSTS_PATH"); ok {
		path = val
	}

	return path
}

func NewHostsSection(name string, file *AtomicFile) (*HostsSection, error) {
	return &HostsSection{
		name:  name,
		hosts: make(map[string][]string),
		file:  file,
	}, nil
}

func (s *HostsSection) Add(address string, hosts ...string) {
	s.hosts[address] = hosts
}

func (s *HostsSection) Remove(address string) {
	delete(s.hosts, address)
}

func (s *HostsSection) Clear() {
	clear(s.hosts)
}

func (s *HostsSection) Flush() error {
	ln := "\n"

	if runtime.GOOS == "windows" {
		ln = "\r\n"
	}

	data, err := s.file.ReadAll()

	if err != nil {
		return err
	}

	text := string(data)

	headerStart := fmt.Sprintf("# Start Section %s%s", s.name, ln)
	headerEnd := fmt.Sprintf("# End Section %s%s", s.name, ln)

	sectionStart := strings.Index(text, headerStart)
	sectionEnd := strings.LastIndex(text, headerEnd)

	sectionFound := sectionStart >= 0 && sectionEnd > sectionStart

	if !sectionFound && len(s.hosts) == 0 {
		return nil
	}

	if sectionFound {
		text = text[:sectionStart] + text[sectionEnd+len(headerEnd):]
	}

	if len(s.hosts) > 0 {
		text = strings.TrimRight(text, ln) + ln + ln
		text += headerStart

		for address, hosts := range s.hosts {
			slices.Sort(hosts)

			for _, host := range hosts {
				text += fmt.Sprintf("%s %s%s", address, host, ln)
			}
		}

		text += fmt.Sprintf("%s%s", headerEnd, ln)
	}

	text = strings.TrimRight(text, ln) + ln

	if _, err := s.file.WriteString(text); err != nil {
		return err
	}

	return nil
}
