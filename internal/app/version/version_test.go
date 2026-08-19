package version

import (
	"strings"
	"testing"
)

// A stamped build says its tag and nothing else, because that is what goes on
// a release page and into a bug report.
func TestStampedBuildSaysItsTag(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	Version = "v1.2.3"
	if got := String(); got != "v1.2.3" {
		t.Errorf("String() = %q, want v1.2.3", got)
	}
	if d := Detail(); !strings.HasPrefix(d, "v1.2.3") {
		t.Errorf("Detail() = %q, want it to start with the tag", d)
	}
}

// An unstamped build names the commit rather than saying "dev" and leaving
// somebody to ask which one.
func TestUnstampedBuildNamesTheCommit(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	Version = ""
	got := String()
	rev, _, ok := vcs()
	if !ok {
		// go test builds do carry VCS stamps, but not from every environment.
		if got != "dev" {
			t.Errorf("String() = %q with no VCS info, want dev", got)
		}
		return
	}
	if !strings.HasPrefix(got, "dev+") || !strings.Contains(got, rev) {
		t.Errorf("String() = %q, want dev+%s with an optional -dirty", got, rev)
	}
	if len(rev) != 7 {
		t.Errorf("revision %q is %d characters, want 7 - a status bar has no room for forty", rev, len(rev))
	}
}

// Detail carries the Go version, which is the second thing anyone asks.
func TestDetailNamesTheToolchain(t *testing.T) {
	if d := Detail(); !strings.Contains(d, "go1.") {
		t.Errorf("Detail() = %q, want a Go version in it", d)
	}
}
