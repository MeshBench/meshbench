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

// modulePath is this repository's import path, which is how a program that
// imports the client rather than running the binary finds out what it pulled
// in: the linker stamp below only ever reaches a binary this repository builds.
const modulePath = "github.com/MeshBench/meshbench"

// Release is the release this build belongs to, spelled the way every artefact
// of that release spells it, and empty for anything that is not one.
//
// Empty is the answer for a working copy, and that is the point: it is what
// lets the control socket tell a released pair, which must match, from a build
// somebody is developing, which cannot be expected to.
//
// Only a plain X.Y.Z counts. The release pipeline stamps "v1.2.3" here, PyPI
// and npm carry "1.2.3" for the same release, and a module pulled from a
// commit rather than a tag carries a pseudo-version. One spelling, or nothing:
// a number that is nearly right would refuse a pair that is fine.
func Release() string {
	if r := plainRelease(Version); r != "" {
		return r
	}
	return plainRelease(moduleRelease())
}

// moduleRelease is the version of this module that the program importing it was
// built against. A script that `go get`s the Go client gets its version from the
// module graph and never sees a linker flag, so this is the only place a
// released client is recognisable as one.
func moduleRelease() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, d := range bi.Deps {
		if d.Path != modulePath {
			continue
		}
		// A replaced module is somebody's working copy, whatever the require
		// line above it happens to say. That line is routinely a placeholder -
		// v0.0.0 is the conventional one - and taking it at face value would
		// have a developer's own checkout announce itself as release 0.0.0 and
		// be refused by every workbench it was pointed at.
		if d.Replace != nil {
			return ""
		}
		return d.Version
	}
	return ""
}

// plainRelease keeps X.Y.Z, with or without a leading v, and rejects everything
// else - a pseudo-version, a release candidate, "(devel)", the empty string.
func plainRelease(v string) string {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return ""
	}
	for _, p := range parts {
		if p == "" || strings.TrimLeft(p, "0123456789") != "" {
			return ""
		}
	}
	return v
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
