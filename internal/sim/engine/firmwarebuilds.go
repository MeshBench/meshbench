// What firmware is running, and which build it came from.
//
// Split from firmware.go, which is about *starting* MeshCore on a network -
// resolving builds, spreading boot times, waiting for attach. This is the
// other half: answering what came up and what it was built from, which is
// what the library panel and every "which binary produced this result"
// question read.
package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// FirmwareCount is how many nodes are running a real build.
//
// A node the run has dropped does not count. It used to: a process that had
// gone still had its Firmware set, because markFirmwareDown records the name
// and leaves the field alone so the bridge can still be closed. So the count
// stayed at its full figure for the whole life of a run in which a node was
// dead from the first tick - and firmware.state is what a script waits on
// before it measures anything, so the answer to "is the mesh up" was yes on a
// mesh that was not. Four minutes of a fifty-six node figure, with one of the
// fifty-six aborted at hand-over, is how it was found.
func (e *Engine) FirmwareCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, node := range e.nodes {
		if node.Firmware != nil && !e.firmwareDown[node.specRef().Name] {
			n++
		}
	}
	return n
}

// NodeByName finds a node, for the console and the inspector.
func (e *Engine) NodeByName(name string) (*Node, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, n := range e.nodes {
		if n.specRef().Name == name {
			return n, true
		}
	}
	return nil, false
}

// Build is one firmware binary this run attached, with enough to prove it.
//
// A result is only interpretable if you know which binary produced it. Naming
// the version is not enough: two runs can name the same version and resolve to
// different files, and two arms can name different versions and resolve to the
// same file. The path and a checksum settle both cases without anybody having
// to reconstruct what was on disk at the time.
type Build struct {
	// Key is role@version, as the resolver was asked for it.
	Key  string
	Path string
	// Sum is the first 12 hex digits of the binary's SHA-256.
	Sum string
}

// Builds is what the current firmware attach resolved to.
func (e *Engine) Builds() []Build {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Build, len(e.builds))
	copy(out, e.builds)
	return out
}

// shortSum hashes a binary so two runs can be compared without keeping it.
func shortSum(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}
