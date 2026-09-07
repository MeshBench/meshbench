package version

import "testing"

// A development build is a release of its own on the development channel. It
// used to read as unreleased - plainRelease rejected any suffix - and a build
// that is not a release pairs with anything, so a stable client could drive a
// development workbench with nothing said.
func TestAPreReleaseIsAReleaseWithItsSuffixKept(t *testing.T) {
	for in, want := range map[string]string{
		"v0.0.11":                "0.0.11",
		"0.0.11":                 "0.0.11",
		"v0.0.11-dev.3":          "0.0.11-dev.3",
		"v1.2.3-rc.1":            "1.2.3-rc.1",
		"v0.0.11-":               "",
		"v0.0.11-dev..3":         "",
		"v0.0.11-dev 3":          "",
		"(devel)":                "",
		"v0.0.0-20260907-abcdef": "", // a pseudo-version is not a release
		"":                       "",
	} {
		if got := plainRelease(in); got != want {
			t.Errorf("plainRelease(%q) = %q, want %q", in, got, want)
		}
	}
}

// Which channel a build is on follows from its tag alone.
func TestTheChannelFollowsTheTag(t *testing.T) {
	was := Version
	defer func() { Version = was }()

	Version = "v0.0.11"
	if Channel() != "stable" || IsDevelopment() {
		t.Errorf("a plain tag is on %q", Channel())
	}
	Version = "v0.0.11-dev.3"
	if Channel() != "development" || !IsDevelopment() {
		t.Errorf("a pre-release tag is on %q", Channel())
	}
	// A working copy is treated as development: it must never be told it is
	// behind a stable release, and it is what an unstamped build most is.
	Version = ""
	if Channel() != "development" {
		t.Errorf("a working copy is on %q, want development", Channel())
	}
}
