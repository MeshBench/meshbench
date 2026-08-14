package ui

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/provider"
)

// The two halves of a scoped mesh spell a region differently, and the mismatch
// is silent at both ends.
//
// A repeater is configured with the bare name (`region put sco`); the key
// travelling on the wire is a hash of the canonical "#sco". Ask a companion for
// scope "sco" and it keys its packets sha256("sco"), which matches no repeater
// anywhere - they all receive the packet, derive a different key, and decline to
// relay. Eight transmissions, zero relays, and a mesh that looks like it has no
// propagation at all.
func TestScopeIsCanonicalisedBeforeItsKeyIsDerived(t *testing.T) {
	if got := canonicalScope("sco"); got != "#sco" {
		t.Errorf("canonicalScope(%q) = %q, want %q", "sco", got, "#sco")
	}
	if got := canonicalScope("#sco"); got != "#sco" {
		t.Errorf("already-canonical scope was changed: got %q", got)
	}
	// The bug in one line: written either way, the key must come out the same,
	// because the repeater derives only one of them.
	if provider.RegionKey(canonicalScope("sco")) != provider.RegionKey(canonicalScope("#sco")) {
		t.Error("the same region written two ways produced two keys; " +
			"every packet from one would be dropped by repeaters holding the other")
	}
	// And it must still differ from the un-prefixed hash, or the fix is a no-op
	// that happens to pass.
	if provider.RegionKey("sco") == provider.RegionKey("#sco") {
		t.Error("sha256 of the two forms collided; this test proves nothing")
	}
}

// An empty scope means "unscoped" and must not become "#".
func TestEmptyScopeStaysEmpty(t *testing.T) {
	if got := canonicalScope(""); got != "" {
		t.Errorf("canonicalScope(\"\") = %q, want \"\" - unscoped is not a region", got)
	}
}
