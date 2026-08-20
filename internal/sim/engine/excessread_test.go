package engine_test

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// Calibration changes a constant, not the matrix.
//
// The excess-loss term is applied where the cache is read, never stored in
// it - so two engines sharing one measured matrix but carrying different
// terms answer exactly the term apart, and neither walks a single profile
// for the difference. Baked into the cache, the term made every calibration
// throw away half an hour of ground-walking to move a number every path
// shares.
func TestExcessLossIsReadNotStored(t *testing.T) {
	build := func(excess float64) *engine.Engine {
		e := engine.New(flat{100}, engine.Config{
			StepMs: 10, Seed: 4417, ExcessPathLossDB: excess})
		e.Add(node("a", 56.20, -3.16, 22), nil)
		e.Add(node("b", 56.23, -3.10, 22), nil)
		return e
	}
	base := build(0)
	l0, ok := base.PathLossForTest(0, 1)
	if !ok {
		t.Fatal("no path loss at all")
	}

	shifted := build(23.5)
	// The measured matrix moves between engines untouched, exactly as the
	// on-disk cache does across a calibrate-rebuild.
	shifted.RestoreLinkCache(base.LinkCacheSnapshot())
	before := shifted.LiveProfiles()
	l1, ok := shifted.PathLossForTest(0, 1)
	if !ok {
		t.Fatal("the restored matrix answered nothing")
	}
	if shifted.LiveProfiles() != before {
		t.Fatal("changing the excess term re-walked a profile; the whole point " +
			"of read-side application is that it must not")
	}
	if d := l1 - l0; math.Abs(d-23.5) > 1e-9 {
		t.Fatalf("the term moved the loss by %.3f dB, want exactly 23.5", d)
	}
}

// A pair beyond the horizon is culled without a single elevation lookup.
//
// An import that carries one stray node on another continent used to have a
// hundred thousand kilometres of profile walked against it - and, once the
// warm learned to prefetch, the Sahara downloaded under it. The Earth's own
// bulge is a floor under any terrain, so the refusal needs no terrain at all.
func TestAContinentalPairIsCulledWithoutWalking(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417})
	e.Add(node("fife", 56.20, -3.16, 22), nil)
	e.Add(node("equator", 0.5, 6.0, 22), nil)
	before := e.LiveProfiles()
	if _, ok := e.PathLossForTest(0, 1); !ok {
		t.Fatal("a culled pair still answers, with its free-space underestimate")
	}
	if e.LiveProfiles() != before {
		t.Fatal("a pair the planet refuses was still walked")
	}
}
