package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func hostsFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "hosts")

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write hosts file: %v", err)
	}

	t.Setenv("HOSTS_PATH", path)

	return path
}

func section(t *testing.T, name string) *HostsSection {
	t.Helper()

	s, err := NewHostsSection(name)

	if err != nil {
		t.Fatalf("NewHostsSection(%q): %v", name, err)
	}

	return s
}

// The catapult and the gateway own separate sections of the same file and flush
// on their own timers, so the read-modify-write has to hold one lock throughout:
// otherwise the later writer rebuilds the file from a snapshot that predates the
// earlier one and reverts its section.
//
// Each writer publishes a different hostname every round, so a write built on a
// stale snapshot leaves the other section showing a hostname it has already
// moved on from.
func TestHostsSectionConcurrentFlush(t *testing.T) {
	const rounds = 300

	path := hostsFile(t, "127.0.0.1 localhost\n")

	// Built on the test goroutine: section calls t.Setenv, which may not run
	// concurrently with the test.
	a := section(t, "A")
	b := section(t, "B")

	var wg sync.WaitGroup

	write := func(s *HostsSection, address, prefix string) {
		defer wg.Done()

		for i := range rounds {
			s.Clear()
			s.Add(address, fmt.Sprintf("%s%d.example.com", prefix, i))

			if err := s.Flush(); err != nil {
				t.Errorf("flush %s round %d: %v", s.name, i, err)
				return
			}
		}
	}

	wg.Add(2)

	go write(a, "127.0.1.1", "a")
	go write(b, "127.0.2.1", "b")

	wg.Wait()

	data, err := os.ReadFile(path)

	if err != nil {
		t.Fatalf("read hosts file: %v", err)
	}

	text := string(data)

	want := []string{
		"127.0.0.1 localhost",
		fmt.Sprintf("127.0.1.1 a%d.example.com", rounds-1),
		fmt.Sprintf("127.0.2.1 b%d.example.com", rounds-1),
	}

	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Fatalf("concurrent flushes lost %q:\n%s", w, text)
		}
	}
}

// The file is rewritten on every refresh, so an unchanged section has to render
// byte-identical rather than reshuffling with map order.
func TestHostsSectionFlushIsStable(t *testing.T) {
	path := hostsFile(t, "127.0.0.1 localhost\n")

	s := section(t, "Loop")

	for _, addr := range []string{"127.0.1.1", "127.0.1.2", "127.0.1.3", "127.0.1.4", "127.0.1.5"} {
		s.Add(addr, "b."+addr+".test", "a."+addr+".test")
	}

	if err := s.Flush(); err != nil {
		t.Fatalf("first flush: %v", err)
	}

	first, err := os.ReadFile(path)

	if err != nil {
		t.Fatalf("read hosts file: %v", err)
	}

	for i := range 20 {
		if err := s.Flush(); err != nil {
			t.Fatalf("flush %d: %v", i, err)
		}

		next, err := os.ReadFile(path)

		if err != nil {
			t.Fatalf("read hosts file: %v", err)
		}

		if string(next) != string(first) {
			t.Fatalf("flush %d changed the file:\n%s\nwant:\n%s", i, next, first)
		}
	}
}

// Add takes a slice the caller keeps using — a tunnel's own host list — so
// rendering must not reorder it underneath them.
func TestHostsSectionAddDoesNotMutateCaller(t *testing.T) {
	hostsFile(t, "127.0.0.1 localhost\n")

	s := section(t, "Loop")

	hosts := []string{"b.test", "a.test"}
	s.Add("127.0.1.1", hosts...)

	if err := s.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if hosts[0] != "b.test" || hosts[1] != "a.test" {
		t.Fatalf("caller's slice was reordered: %v", hosts)
	}
}

// Clearing a section must leave the rest of the file, and any other section,
// untouched.
func TestHostsSectionLeavesOtherContentAlone(t *testing.T) {
	path := hostsFile(t, "127.0.0.1 localhost\n255.255.255.255 broadcasthost\n")

	a := section(t, "A")
	b := section(t, "B")

	a.Add("127.0.1.1", "a.example.com")
	b.Add("127.0.2.1", "b.example.com")

	if err := a.Flush(); err != nil {
		t.Fatalf("flush A: %v", err)
	}

	if err := b.Flush(); err != nil {
		t.Fatalf("flush B: %v", err)
	}

	a.Clear()

	if err := a.Flush(); err != nil {
		t.Fatalf("re-flush A: %v", err)
	}

	data, err := os.ReadFile(path)

	if err != nil {
		t.Fatalf("read hosts file: %v", err)
	}

	text := string(data)

	if strings.Contains(text, "a.example.com") {
		t.Fatalf("cleared section still present:\n%s", text)
	}

	for _, want := range []string{"127.0.0.1 localhost", "255.255.255.255 broadcasthost", "127.0.2.1 b.example.com"} {
		if !strings.Contains(text, want) {
			t.Fatalf("clearing section A dropped %q:\n%s", want, text)
		}
	}
}
