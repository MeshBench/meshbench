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

// Does the sweep's own honesty check know that its arms cannot be compared?
//
// This is the check that already refuses to call one seed a spread, and an
// emulated arm is the stronger version of the same objection: the seed cannot
// bound noise that did not come from the seed. It has to outrank the others,
// because they are about how large a difference has to be to count and this one
// is about whether the difference came from the arm at all.
func TestASweepOverAnEmulatedNodeIsNotAComparison(t *testing.T) {
	e := &experiment{
		Arms:            []ExpArm{{Label: "a"}, {Label: "b"}},
		Seeds:           []uint64{1, 2, 3},
		results:         []ExpResult{{Arm: "a", Seed: 1}},
		notReproducible: "kelpie runs in an emulator",
	}
	warn := e.notAResultYet()
	if !strings.Contains(warn, "not comparable") || !strings.Contains(warn, "kelpie") {
		t.Errorf("a sweep over an emulated node warned %q", warn)
	}

	// And the same matrix over native nodes gets past this objection to the
	// ones about seeds and arms, rather than being warned about for ever.
	e.notReproducible = ""
	if warn := e.notAResultYet(); strings.Contains(warn, "not comparable") {
		t.Errorf("a native sweep was told its arms are not comparable: %q", warn)
	}
}
