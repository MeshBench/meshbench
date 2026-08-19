// Package version is what this build is.
//
// One answer, because there were three: the release pipeline stamped a
// variable in the workbench, and both commands carried a `const version =
// "0.1.0"` that no release ever touched. A tagged build reported 0.1.0 from
// the command line and its real version in the licence window, which is the
// kind of disagreement that surfaces in a bug report about something else.
package version

import (
	"runtime/debug"
	"strings"
)

// Version is stamped by the release pipeline:
//
//	-ldflags "-X github.com/MeshBench/meshbench/internal/app/version.Version=v1.2.3"
//
// Empty in every other build, which is not a failure - it is how String tells
// a release from a working copy.
var Version = ""

// String is the short form, for a status bar or a banner.
//
// A stamped release says its tag. Anything else says so and names the commit,
// because "dev" alone on a screenshot in a bug report costs a round trip.
func String() string {
	if Version != "" {
		return Version
	}
	rev, dirty, ok := vcs()
	if !ok {
		return "dev"
	}
	s := "dev+" + rev
	if dirty {
		s += "-dirty"
	}
	return s
}

// Detail is the long form, for `--version` and anywhere with room.
func Detail() string {
	parts := []string{String()}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.GoVersion != "" {
		parts = append(parts, bi.GoVersion)
	}
	if _, dirty, ok := vcs(); ok && dirty && Version != "" {
		// A stamped build from a dirty tree is a release nobody can reproduce.
		parts = append(parts, "built from modified sources")
	}
	return strings.Join(parts, ", ")
}

// vcs reads what the toolchain stamped in. Go records the revision and whether
// the tree was clean for any build from a repository, so a developer build can
// still say exactly what it is without the release pipeline being involved.
func vcs() (rev string, dirty, ok bool) {
	bi, found := debug.ReadBuildInfo()
	if !found {
		return "", false, false
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "", false, false
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	return rev, dirty, true
}
