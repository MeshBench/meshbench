package session

// White-box, because the fact under test is what the session was built out of:
// a Sim holding nodes is the cheapest honest way to ask a verb what it makes of
// a network with an emulator in it.

import (
	"context"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// stateOf opens a session on the given Sim and asks it where the run has got to.
func stateOf(t *testing.T, s *Sim) map[string]any {
	t.Helper()
	store := state.New(10)
	Register(store, s)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go store.Run(ctx)
	reply, err := store.Do(ctx, "sim.state", nil)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := reply.(map[string]any)
	if out == nil {
		t.Fatalf("sim.state answered %T", reply)
	}
	return out
}

// Does the verb a script polls say whether its answers can be repeated?
//
// It did not, and the seed sitting one key above it made that worse rather
// than neutral: a seed reads as a promise that the run can be done again and
// diffed. On a scenario carrying an emulated node that promise is false, and
// the failure is silent - two runs come back with different instants and
// nothing anywhere says which of the two numbers to believe.
func TestSimStateSaysWhenARunCannotBeRepeated(t *testing.T) {
	got := stateOf(t, simWithBoards("Heltec_v3"))
	if ok, _ := got["reproducible"].(bool); ok {
		t.Error("a scenario with an emulated node reported itself reproducible")
	}
	why, _ := got["not_reproducible_why"].(string)
	if !strings.Contains(why, "emulator") {
		t.Errorf("sim.state gave no usable reason: %q", why)
	}
}

// And says nothing where there is nothing to say. A caveat on every run is a
// caveat nobody reads on the run that has one.
func TestSimStateKeepsTheGuaranteeForANativeMesh(t *testing.T) {
	got := stateOf(t, simWithBoards("", ""))
	if ok, _ := got["reproducible"].(bool); !ok {
		t.Error("a native-only scenario reported itself not reproducible")
	}
	if why, _ := got["not_reproducible_why"].(string); why != "" {
		t.Errorf("a native-only scenario gave a reason anyway: %q", why)
	}
}
