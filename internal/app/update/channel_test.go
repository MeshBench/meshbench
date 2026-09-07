package update

import "testing"

// The ordering is semver's, and every line of it matters to somebody: a
// development build must be offered the next development build, must be
// offered the stable release that closes its series, and a stable build must
// never be told a pre-release is newer than it.
func TestNewerOrdersPreReleasesTheWaySemverDoes(t *testing.T) {
	for _, tc := range []struct {
		build, latest string
		want          bool
	}{
		{"0.0.10", "0.0.11", true},
		{"0.0.11", "0.0.10", false},
		{"0.0.10", "0.0.11-dev.1", true},  // a dev build of the next version is newer
		{"0.0.11", "0.0.11-dev.9", false}, // a release is never behind its own pre-releases
		{"0.0.11-dev.3", "0.0.11", true},  // and a pre-release is behind the release it precedes
		{"0.0.11-dev.3", "0.0.11-dev.4", true},
		{"0.0.11-dev.4", "0.0.11-dev.3", false},
		{"0.0.11-dev.9", "0.0.11-dev.10", true}, // numbers as numbers
		{"0.0.11-dev.3", "0.0.11-dev.3", false},
		{"", "0.0.11-dev.1", false}, // a working copy is never behind
		{"0.0.11-dev.3", "0.0.12", true},
	} {
		if got := Newer(tc.build, tc.latest); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.build, tc.latest, got, tc.want)
		}
	}
}

// A pseudo-version is a working copy with a longer name, not a pre-release.
func TestAPseudoVersionIsNotARelease(t *testing.T) {
	if _, _, ok := parse("v0.0.0-20240101120000-abcdef123456"); ok {
		t.Error("a pseudo-version parsed as a release")
	}
	if _, pre, ok := parse("v0.0.11-dev.3"); !ok || pre != "dev.3" {
		t.Errorf("v0.0.11-dev.3 parsed as ok=%v pre=%q", ok, pre)
	}
}
