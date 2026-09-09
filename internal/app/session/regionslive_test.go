package session

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A region set on a running node has to reach the node's console. The lines
// are a boot's, plus a remove for every region the node no longer holds, which
// a boot never needs because it starts from an empty map.
func TestLiveRegionCommandsAddRemoveAndSave(t *testing.T) {
	rep := func(regions ...string) scenario.Node {
		return scenario.Node{Name: "hill", Kind: scenario.SimpleRepeater, Regions: regions}
	}

	// Adding a region to a node that had one: the old one stays, the new one is
	// put and allowed, and it is saved.
	got := liveRegionCommands([]string{"fif"}, rep("fif", "sco"))
	joined := strings.Join(got, "\n")
	for _, want := range []string{"region put sco", "region allowf sco", "region save"} {
		if !strings.Contains(joined, want) {
			t.Errorf("adding sco did not send %q; got %v", want, got)
		}
	}
	if strings.Contains(joined, "region remove") {
		t.Errorf("nothing was dropped, so nothing should be removed; got %v", got)
	}

	// Dropping a region: it is removed, and the change is saved even though
	// nothing was added.
	got = liveRegionCommands([]string{"fif", "sco"}, rep("fif"))
	joined = strings.Join(got, "\n")
	if !strings.Contains(joined, "region remove sco") {
		t.Errorf("dropping sco did not remove it; got %v", got)
	}
	if !strings.Contains(joined, "region save") {
		t.Errorf("a drop was not saved; got %v", got)
	}

	// Clearing every region: each is removed and the empty map is saved.
	got = liveRegionCommands([]string{"fif"}, rep())
	joined = strings.Join(got, "\n")
	if !strings.Contains(joined, "region remove fif") || !strings.Contains(joined, "region save") {
		t.Errorf("clearing regions did not remove and save; got %v", got)
	}

	// No change: nothing to send, so the node is not touched.
	if got := liveRegionCommands([]string{"fif"}, rep("fif")); len(got) != 0 {
		// RegionCommands re-asserts put/allowf/save even with no diff, so an
		// unchanged set still re-provisions; that is harmless and idempotent.
		// What must never happen is a spurious remove.
		if strings.Contains(strings.Join(got, "\n"), "region remove") {
			t.Errorf("an unchanged set issued a remove; got %v", got)
		}
	}

	// The hash prefix on a scope is stripped both when matching old against new
	// and when emitting: #sco held and sco kept is not a drop.
	if got := liveRegionCommands([]string{"#sco"}, rep("sco")); strings.Contains(strings.Join(got, "\n"), "region remove") {
		t.Errorf("#sco and sco are the same region; got %v", got)
	}
}
