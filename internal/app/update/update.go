// Package update is whether a newer release exists, and getting it onto the
// disk beside this one.
//
// It does not replace anything. A release here is not only the binary: it pins
// the emulator toolchain, the per-role firmware tags and the fixtures, so a
// month-old build fetching this month's published firmware is a combination
// nobody tested and nothing said so. What this package adds is the telling, and
// a verified download; the swap stays a thing a person decides to do, because
// the alternative is replacing a binary underneath a run holding unsaved state.
//
// Nothing here starts on its own. Every entry point takes a context and a
// caller, and the caller is the one that has been told it may spend bandwidth.
package update

import (
	"strconv"
	"strings"
	"time"
)

// Release is one published release, as the feed describes it.
type Release struct {
	// Tag is what the release is called - "v0.2.0" - and Version is the same
	// number spelled the way every other artefact of a release spells it,
	// empty for anything that is not a plain X.Y.Z.
	Tag     string
	Version string
	// Notes is the release page. Prose outgrows any panel, so it is linked
	// rather than embedded.
	Notes     string
	Published time.Time
	// Prerelease is carried rather than acted on: the feed this asks is
	// GitHub's "latest", which never answers with one. It is here so a
	// hand-pointed feed that does can be refused rather than silently taken.
	Prerelease bool
	Assets     []Asset
}

// Asset is one file published with a release.
type Asset struct {
	Name  string
	URL   string
	Bytes int64
}

// Find returns the named asset. Names are unique within a release.
func (r Release) Find(name string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// Newer reports whether latest is a higher version than build.
//
// Both have to be plain X.Y.Z. A working copy's version is empty and gets
// false, which is the whole point: it is not behind, it is unreleased, and
// telling somebody who is building the thing that they are out of date is how
// an update check earns its reputation.
func Newer(build, latest string) bool {
	b, ok := triple(build)
	if !ok {
		return false
	}
	l, ok := triple(latest)
	if !ok {
		return false
	}
	for i := range b {
		if l[i] != b[i] {
			return l[i] > b[i]
		}
	}
	return false
}

// triple parses X.Y.Z, with or without a leading v, and refuses everything
// else - a pseudo-version, a release candidate, the empty string.
func triple(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
