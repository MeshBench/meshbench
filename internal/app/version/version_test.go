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

// The release is the number the control socket pairs a client to a workbench
// by, so it has to be spelled the one way every artefact of a release spells
// it: the linker stamps "v1.2.3" and PyPI and npm carry "1.2.3" for the same
// release, and a comparison that kept the v would refuse every released pair.
func TestReleaseIsSpelledTheWayEveryArtefactSpellsIt(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	Version = "v1.2.3"
	if got := Release(); got != "1.2.3" {
		t.Errorf("Release() = %q, want 1.2.3", got)
	}
	Version = "1.2.3"
	if got := Release(); got != "1.2.3" {
		t.Errorf("Release() = %q, want 1.2.3", got)
	}
}

// Empty is the answer for anything that is not a release, and it is the answer
// the pairing rule reads as "there is nothing here to disagree with". A number
// that is nearly right would refuse a pair that is fine, so only a plain X.Y.Z
// counts: not a pseudo-version, not a release candidate, not "(devel)".
func TestOnlyAPlainReleaseCounts(t *testing.T) {
	for _, v := range []string{
		"", "dev", "(devel)", "v1.2", "v1.2.3.4", "v1.2.3-rc.1",
		"v0.0.0-20240101120000-abcdef123456", "v1.2.x",
	} {
		if got := plainRelease(v); got != "" {
			t.Errorf("plainRelease(%q) = %q, want it not to count as a release", v, got)
		}
	}
	if got := plainRelease("v10.20.30"); got != "10.20.30" {
		t.Errorf("plainRelease(v10.20.30) = %q, want 10.20.30", got)
	}
}

// A working copy stamps nothing, and that is what tells the control socket it
// is looking at a build somebody is developing rather than at half of a
// released pair.
func TestAWorkingCopyIsNotARelease(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	Version = ""
	// The test binary's own module is the main module, not a dependency, so
	// there is nothing in the module graph to fall back to either.
	if got := Release(); got != "" {
		t.Errorf("Release() = %q from an unstamped build, want empty", got)
	}
}

// Detail carries the Go version, which is the second thing anyone asks.
func TestDetailNamesTheToolchain(t *testing.T) {
	if d := Detail(); !strings.Contains(d, "go1.") {
		t.Errorf("Detail() = %q, want a Go version in it", d)
	}
}
