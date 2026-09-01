package session

import (
	"errors"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// aNetwork is the whole verb set over two named nodes.
//
// The names are two real ScotMesh sites and one is nearly the other's typo,
// because "nearly right" is the parameter these verbs used to accept and act
// on: every refusal below is about a name or a number somebody meant.
func aNetwork(t *testing.T) (*state.Store, *Sim) {
	t.Helper()
	st, s := register(t)
	st.Handle("test.nodes", func(w *state.World, p any) (any, error) {
		nodes, ok := p.([]state.Node)
		if !ok {
			t.Fatalf("test.nodes was handed %T", p)
		}
		w.Nodes = nodes
		return nil, nil
	})
	go st.Run(t.Context())

	s.nodes = []scenario.Node{
		{Name: "West Lomond", Kind: scenario.SimpleRepeater},
		{Name: "Dunfermline", Kind: scenario.SimpleRepeater},
	}
	s.nodes[0].Position.Lat, s.nodes[0].Position.Lon = 56.25, -3.29
	s.nodes[1].Position.Lat, s.nodes[1].Position.Lon = 56.07, -3.46
	if _, err := st.Do(t.Context(), "test.nodes", stateNodes(s.nodes)); err != nil {
		t.Fatal(err)
	}
	return st, s
}

func refuses(t *testing.T, st *state.Store, verb string, params any) string {
	t.Helper()
	got, err := st.Do(t.Context(), verb, params)
	if err == nil {
		t.Fatalf("%s accepted %v and answered %v", verb, params, got)
	}
	return err.Error()
}

func mentions(t *testing.T, msg string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(msg, w) {
			t.Errorf("%q does not mention %q", msg, w)
		}
	}
}

func selected(nodes []state.Node) []string {
	var out []string
	for _, n := range nodes {
		if n.Selected {
			out = append(out, n.Name)
		}
	}
	return out
}

// The shape that started this: {"names": [...]} matched neither branch of the
// type switch, so the verb selected nothing, deselected everything that was
// selected, and answered as though it had done what was asked.
func TestSelectManyTakesTheShapesItAdvertisesAndRefusesTheRest(t *testing.T) {
	st, _ := aNetwork(t)
	ctx := t.Context()

	for _, shape := range []any{
		[]string{"West Lomond"},
		"West Lomond",
		[]any{"West Lomond"},
		map[string]any{"names": []any{"West Lomond"}},
		map[string]any{"names": "West Lomond"},
	} {
		if _, err := st.Do(ctx, "nodes.select_many", shape); err != nil {
			t.Fatalf("nodes.select_many refused %#v: %v", shape, err)
		}
		snap := st.Snapshot()
		if got := selected(snap.Nodes); len(got) != 1 || got[0] != "West Lomond" {
			t.Errorf("%#v selected %v, want just West Lomond", shape, got)
		}
	}

	// A shape nothing recognises is refused, and the selection it would have
	// wiped is still there.
	msg := refuses(t, st, "nodes.select_many", map[string]any{"nodes": []any{"West Lomond"}})
	mentions(t, msg, "nodes.select_many", "names")
	if got := selected(st.Snapshot().Nodes); len(got) != 1 {
		t.Errorf("the refused call changed the selection to %v", got)
	}

	msg = refuses(t, st, "nodes.select_many", 42)
	mentions(t, msg, "nodes.select_many")

	// One bad member fails the list rather than being dropped: selecting the
	// other thirty-nine is a different answer to the question asked.
	msg = refuses(t, st, "nodes.select_many", []any{"West Lomond", 7})
	mentions(t, msg, "nodes.select_many", "member 1")
}

// A name the network has not got is a typo or a stale script, and both need to
// be told which names it does have.
func TestSelectionVerbsRefuseNamesTheNetworkHasNot(t *testing.T) {
	st, _ := aNetwork(t)
	for _, verb := range []string{"nodes.select_many", "nodes.add_to_selection"} {
		msg := refuses(t, st, verb, []string{"West Lomand"})
		mentions(t, msg, verb, "West Lomand", "West Lomond", "Dunfermline")
	}
}

// Clearing the selection is what no names means, and it has to keep working:
// this is the one legitimate empty.
func TestSelectManyStillClearsTheSelection(t *testing.T) {
	st, _ := aNetwork(t)
	ctx := t.Context()
	if _, err := st.Do(ctx, "nodes.select_many", []string{"West Lomond"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Do(ctx, "nodes.select_many", nil); err != nil {
		t.Fatalf("clearing the selection was refused: %v", err)
	}
	if got := selected(st.Snapshot().Nodes); len(got) != 0 {
		t.Errorf("still selected: %v", got)
	}
}

// A missing or mistyped coordinate used to come back as zero, so the node went
// to the Gulf of Guinea and the move reported the position it had invented.
func TestMoveRefusesACoordinateItCannotRead(t *testing.T) {
	st, _ := aNetwork(t)
	for _, c := range []struct {
		what   string
		params any
		says   []string
	}{
		{"no lon at all", map[string]any{"name": "West Lomond", "lat": 56.2},
			[]string{"nodes.move", "lon"}},
		{"a lat that is text", map[string]any{
			"name": "West Lomond", "lat": "56.2", "lon": -3.3},
			[]string{"nodes.move", "lat"}},
		{"a lat off the globe", map[string]any{
			"name": "West Lomond", "lat": 560.0, "lon": -3.3},
			[]string{"nodes.move", "lat", "-90", "90"}},
		{"no name", map[string]any{"lat": 56.2, "lon": -3.3},
			[]string{"nodes.move", "name"}},
		{"not an object at all", "West Lomond", []string{"nodes.move"}},
	} {
		msg := refuses(t, st, "nodes.move", c.params)
		mentions(t, msg, c.says...)
		if strings.Contains(msg, "nodes.move") == false {
			t.Errorf("%s: %q does not name the verb", c.what, msg)
		}
	}

	// Zero is a coordinate like any other, and the two of them together are a
	// real place in the Atlantic. It must not read as "not given".
	if _, err := st.Do(t.Context(), "nodes.move",
		map[string]any{"name": "West Lomond", "lat": 0.0, "lon": 0.0}); err != nil {
		t.Fatalf("a node could not be moved to null island: %v", err)
	}
}

// A typo used to originate the packet at whichever node happened to be first
// and report that as success, which is the worst possible answer: the run looks
// like the run that was asked for and is not.
func TestInjectRefusesANodeThisNetworkHasNot(t *testing.T) {
	st, _ := aNetwork(t)
	msg := refuses(t, st, "sim.inject", "West Lomand")
	mentions(t, msg, "sim.inject", "West Lomand", "West Lomond", "Dunfermline")

	// A name that does exist gets past the name check and stops at the engine,
	// which is the next thing this session has not got. Proving the refusal is
	// about the name and not about everything.
	_, err := st.Do(t.Context(), "sim.inject", "West Lomond")
	if !errors.Is(err, ErrNoSimulation) {
		t.Fatalf("a node that does exist was refused with %v", err)
	}
}

// The failure used to be delivered to node.reflash_failed, which the caller
// that asked never subscribes to - and the client façade spells this as an
// assignment, so the assignment appeared to work.
func TestSetFirmwareRefusesItsCallerRatherThanACallbackNobodyReads(t *testing.T) {
	st, _ := aNetwork(t)
	for _, verb := range []string{"node.set_firmware", "node.set_firmware_only"} {
		msg := refuses(t, st, verb, map[string]any{
			"node": "West Lomand", "version": "1.7.0"})
		mentions(t, msg, "West Lomand")

		msg = refuses(t, st, verb, map[string]any{"node": "West Lomond"})
		mentions(t, msg, verb, "version")

		msg = refuses(t, st, verb, map[string]any{"version": "1.7.0"})
		mentions(t, msg, verb, "node")
	}
}

// experiment.stop cannot wait for the run goroutine - the worker reports back
// through this same store, so blocking here would deadlock the thing being
// waited on - which is why a start had to refuse instead. Starting anyway
// cleared the results out from under a worker still appending to them, and the
// new run's table carried the tail of the old one's cells.
func TestAnExperimentWillNotStartOverTheLastOnesTail(t *testing.T) {
	st, s := aNetwork(t)
	e := s.experiment()
	e.mu.Lock()
	e.Senders = []string{"West Lomond"}
	e.Arms = []ExpArm{{Label: "a"}}
	e.Seeds = []uint64{1}
	// A run that has been told to stop and has not yet let go of the results.
	e.running = false
	e.done = make(chan struct{})
	e.mu.Unlock()

	msg := refuses(t, st, "experiment.start", nil)
	mentions(t, msg, "still stopping")

	// stop says so too, rather than claiming the run is over.
	got, err := st.Do(t.Context(), "experiment.stop", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["settled"] != false {
		t.Errorf("stop reported %v, want a run that has not settled", got)
	}
}

// The flag feed.stop clears was written and never read, so the pull it was
// pressed to stop landed a minute later and filled the panel up anyway.
func TestFeedStopIsReadBackByThePullItStops(t *testing.T) {
	st, s := aNetwork(t)
	ctx := t.Context()

	s.feeding.Store(true)
	got, err := st.Do(ctx, "feed.stop", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["stopped"] != true {
		t.Errorf("stopping a running feed answered %v", got)
	}
	if s.feeding.Load() {
		t.Error("the feed is still running")
	}

	// And stopping nothing says so rather than claiming a stop.
	got, err = st.Do(ctx, "feed.stop", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["stopped"] != false {
		t.Errorf("stopping an idle feed answered %v", got)
	}
}
