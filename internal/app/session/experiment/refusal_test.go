// What the matrix refuses, and why it has to refuse rather than wait.
package experiment

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
)

// experiment.stop cannot wait for the run goroutine - the worker reports back
// through this same store, so blocking here would deadlock the thing being
// waited on - which is why a start had to refuse instead. Starting anyway
// cleared the results out from under a worker still appending to them, and the
// new run's table carried the tail of the old one's cells.
func TestAnExperimentWillNotStartOverTheLastOnesTail(t *testing.T) {
	st, s := aNetwork(t)
	e := matrixOf(s)
	e.mu.Lock()
	e.Senders = []string{"West Lomond"}
	e.Arms = []session.ExpArm{{Label: "a"}}
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

// Does the sweep's own honesty check know that its arms cannot be compared?
//
// This is the check that already refuses to call one seed a spread, and an
// emulated arm is the stronger version of the same objection: the seed cannot
// bound noise that did not come from the seed. It has to outrank the others,
// because they are about how large a difference has to be to count and this one
// is about whether the difference came from the arm at all.
func TestASweepOverAnEmulatedNodeIsNotAComparison(t *testing.T) {
	e := &experiment{
		Arms:            []session.ExpArm{{Label: "a"}, {Label: "b"}},
		Seeds:           []uint64{1, 2, 3},
		results:         []Result{{Arm: "a", Seed: 1}},
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
