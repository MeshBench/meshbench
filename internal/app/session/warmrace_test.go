package session

import (
	"context"
	"fmt"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A warm outlives the network it was started for.
//
// Placing a node rebuilds the engine and starts a new warm, and the warm
// before it is still on its own goroutine when that happens. That warm used to
// read the session's live engine and live node list when it came to publish,
// so it measured whatever had been opened since - and during a rebuild those
// two disagree by construction: the node list is replaced in one assignment
// while the new engine is filled a node at a time afterwards. A pair index
// taken from the list against an engine three nodes into its fill is out of
// range, and out of range on a worker goroutine is the whole process, with
// whatever was unsaved in it.
//
// Stress rather than choreography: the window one placement opens is
// microseconds wide, so this holds a single warm and rebuilds underneath it
// until that warm has published. It fails the way the bug fails, with a bounds
// panic, and under -race it fails as a live field read on one goroutine and
// written on another.
func TestAWarmDoesNotFollowTheSessionOntoAnotherNetwork(t *testing.T) {
	st := state.New(10)
	// gpuAsked keeps the GPU out of a test about which network is measured,
	// and spending the probe here stops it opening this machine's real device
	// to answer a question this test never asks. The probe is process-wide, so
	// this is a no-op if something else in the binary has already asked. Bare
	// earth keeps the developer's tile cache out of it for the same reason.
	probeOnce.Do(func() { machineProbe = &gpuProbe{why: "not asked here"} })
	s := &Sim{gpuAsked: true}
	s.terr = bareEarth{}
	Register(st, s)

	nodes := make([]scenario.Node, 0, 300)
	for i := 0; i < 300; i++ {
		n := repeaterNode(fmt.Sprintf("n%03d", i))
		n.Position.Lon += float64(i) / 1000
		nodes = append(nodes, n)
	}
	// A placement, reduced to the half that matters here and put where the
	// real one runs: on the store's goroutine, which is the only thing that
	// ever rebuilds an engine. Rebuilding from the test's own goroutine
	// instead would manufacture a race the workbench does not have, against
	// the verbs the store is answering at the same moment.
	placed := 0
	st.HandleInternal("test.place", func(_ *state.World, _ any) (any, error) {
		placed++
		grown := append(append([]scenario.Node(nil), nodes...),
			repeaterNode(fmt.Sprintf("p%04d", placed)))
		s.buildSeeded(grown, 869.618, defaultSeed)
		return nil, nil
	})

	// The network under test is built before the store is answering anything,
	// so the only rebuilds racing the warm are the placements below.
	s.buildSeeded(nodes, 869.618, defaultSeed)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)
	s.warm(st, len(nodes))

	// The job row rather than s.warmed says when to stop, because a rebuild
	// clears s.warmed itself and this loop would never see it set.
	for i := 0; i < 20000 && !linkJobFinished(st); i++ {
		if _, err := st.Do(ctx, "test.place", nil); err != nil {
			t.Fatalf("placement %d: %v", i, err)
		}
	}
	if !linkJobFinished(st) {
		t.Fatal("the warm never published: twenty thousand placements went by " +
			"and the network it was started for was never measured")
	}
}

// linkJobFinished reports whether the link measurement has published. The job
// id is the warm's own, and only one warm is started here.
func linkJobFinished(st *state.Store) bool {
	for _, j := range st.Snapshot().Jobs {
		if j.ID == "links" && j.Finished {
			return true
		}
	}
	return false
}
