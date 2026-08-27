package sweep

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// The default sweep is four offered-load arms over six seeds at the given node
// - the shape sweep.run hands the engine, pinned here because the plan moved
// out of session into this package.
func TestDefaultSweepShape(t *testing.T) {
	p := DefaultSweep("comp")
	if len(p.Arms) != 4 {
		t.Fatalf("arms = %d, want 4", len(p.Arms))
	}
	if len(p.Seeds) != 6 {
		t.Fatalf("seeds = %d, want 6", len(p.Seeds))
	}
	if p.Node != "comp" {
		t.Fatalf("node = %q, want comp", p.Node)
	}
	if p.Arms[0].EveryMs <= p.Arms[len(p.Arms)-1].EveryMs {
		t.Fatalf("arms should tighten the interval: %d then %d",
			p.Arms[0].EveryMs, p.Arms[len(p.Arms)-1].EveryMs)
	}
}

// FirstCompanion is who a sweep originates from when nothing is selected: the
// first companion, not a repeater or an observer.
func TestFirstCompanionPrefersACompanion(t *testing.T) {
	ns := []scenario.Node{
		{Name: "rep", Kind: scenario.SimpleRepeater},
		{Name: "comp", Kind: scenario.Companion},
	}
	if got := FirstCompanion(ns); got != "comp" {
		t.Fatalf("FirstCompanion = %q, want comp", got)
	}
	if got := FirstCompanion(ns[:1]); got != "rep" {
		t.Fatalf("with no companion, FirstCompanion = %q, want the fallback rep", got)
	}
}
