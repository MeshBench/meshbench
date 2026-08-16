package session

import (
	"context"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// The warm-skip regression: a carried matrix must prime the cache, not
// stand in for having measured it.
//
// buildSeeded used to mark s.warmed true the moment a carried map was
// non-nil - in-process or from disk - whether or not it actually covered
// every pair. A snapshot taken mid-run (radio-state changes evict entries
// live once firmware reports its real configuration) or a disk matrix
// measured against baseline import figures a firmware node can later
// diverge from is neither complete. project.open trusted s.warmed and
// skipped the real warm on the strength of it, so an unmeasured pair was
// left for whichever transmission needed it first - synchronously, on the
// store's own goroutine, which is what "sent an advert and it stopped
// answering, and the console went with it" was.
func TestACarriedMatrixDoesNotSkipTheRealWarm(t *testing.T) {
	s := &Sim{excessLossDB: DefaultExcessLossDB, excessSet: true}
	nodes := []scenario.Node{repeaterNode("Alpha"), repeaterNode("Beta"), repeaterNode("Gamma")}
	// A hundred metres apart: everything hears everything over bare earth,
	// so there is nothing here a warm would find impossible to measure.
	nodes[1].Position.Lon += 0.001
	nodes[2].Position.Lon -= 0.001

	s.build(nodes, 869.618)
	if s.warmed {
		t.Fatal("a fresh build with nothing carried already reported warmed")
	}

	// The partial snapshot a live run leaves behind: one pair cached, not
	// the other two this three-node geometry has.
	s.eng.RestoreLinkCache(map[[2]int]float64{{0, 1}: 100})

	// A rebuild of the identical geometry - what Reset does - carries that
	// partial snapshot forward.
	s.build(nodes, 869.618)
	if s.warmed {
		t.Fatal("a partial carried matrix was trusted as fully warmed - " +
			"play would have started before the missing pairs were measured")
	}
	if !s.cold {
		t.Fatal("a carried matrix left the rebuild looking as though a warm had already run")
	}
}

// The stuck-warming regression: every verb that rebuilds the engine must
// itself call warm, because a rebuild no longer claims a carried matrix is
// complete on its own.
//
// sim.reset called rebuild and returned, and relied on buildSeeded's old
// shortcut to leave the run looking warmed. Once that shortcut was removed,
// nothing after a reset ever called warm again - but s.warmCancel was still
// the (finished) one from before the reset, so s.warming() went on reporting
// true forever: "warming up" at the bottom of the window, play refusing to
// start, no warm actually running to finish it. Only a restart cleared it.
func TestSimResetActuallyRewarms(t *testing.T) {
	st := state.New(10)
	s := &Sim{}
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	nodes := []scenario.Node{repeaterNode("Alpha"), repeaterNode("Beta"), repeaterNode("Gamma")}
	nodes[1].Position.Lon += 0.001
	nodes[2].Position.Lon -= 0.001
	s.build(nodes, 869.618)
	s.warm(st, len(nodes))
	waitWarmed(t, s, "initial warm")

	if _, err := st.Do(ctx, "sim.reset", nil); err != nil {
		t.Fatalf("sim.reset: %v", err)
	}
	waitWarmed(t, s, "warm after sim.reset")
}

// waitWarmed polls rather than sleeping a fixed amount: warm runs on its own
// goroutine, and a fixed sleep either wastes time on a fast machine or is
// exactly the flake this avoids on a slow one.
func waitWarmed(t *testing.T, s *Sim, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s.warmed {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s: never completed - the run would be stuck reporting "+
		"\"warming up\" with nothing left to finish it", what)
}
