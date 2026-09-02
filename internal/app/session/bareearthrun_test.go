// What a run over ground nobody fetched reports about itself.
//
// The bug this pins was silent in the worst way available: a session with no
// terrain marked itself warmed and finished its own job row, so a script that
// opened a fixture, waited for idle and read a result got one. Not an error,
// not an empty answer, not a timeout - a plausible number over free space,
// from a session that said it was ready.
//
// The gate that used to prevent it held the warm until somebody answered a
// question, which made bare earth the resting state of every install nobody
// had answered. Downloads are on by default now, and what has to survive that
// change is this: a run that ends up on bare earth anyway still says so.
package session

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// offlineSession opens a network on a machine with terrain downloads switched
// off, which is the one remaining way to reach bare earth deliberately.
func offlineSession(t *testing.T) (*Sim, *state.Store, context.Context) {
	t.Helper()
	s, st, ctx := consentSim(t)
	if _, err := st.Do(ctx, "terrain.allow", map[string]any{"on": false}); err != nil {
		t.Fatalf("terrain.allow: %v", err)
	}
	if _, err := st.Do(ctx, "project.open", "fem-e22"); err != nil {
		t.Fatalf("project.open: %v", err)
	}
	waitForLinksJob(t, st)
	return s, st, ctx
}

// waitForLinksJob waits until the link measurement has stopped, however it
// stopped. Polled rather than slept on, because the point of the whole test is
// what the job says when it is over.
func waitForLinksJob(t *testing.T, st *state.Store) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		for _, j := range st.Snapshot().Jobs {
			if j.ID == "links" && j.Finished {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the link measurement never finished or failed")
}

// waitForMeasured waits until a warm has actually walked every pair.
func waitForMeasured(t *testing.T, s *Sim) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if s.linksMeasured() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the warm never reported a measured matrix")
}

// Downloads off is an answer, and the warm runs anyway. What comes out of it
// is a measured matrix over bare earth that says so, which is the legitimate
// offline run the switch exists to make possible.
//
// The off case rather than the on one, because on downloads a country: this is
// the half of the pair that touches no network.
func TestAnOfflineMachineStillMeasuresItsLinksAndSaysWhatOver(t *testing.T) {
	s, st, _ := offlineSession(t)
	waitForMeasured(t, s)

	// A decision is what stops a bare-earth run being a silent fallback. It
	// never stops being announced.
	g := st.Snapshot().Ground
	if !g.Bare() || !g.Chosen {
		t.Errorf("a machine with downloads off reports ground %+v", g)
	}
	if !strings.Contains(g.Note, "free space") {
		t.Errorf("a chosen bare-earth run does not say what it means: %q", g.Note)
	}
}

// And the polled verb says the same, because a script never reads a field.
func TestSimStateCarriesTheGroundAndTheMeasurement(t *testing.T) {
	s, st, ctx := offlineSession(t)
	waitForMeasured(t, s)

	v, err := st.Do(ctx, "sim.state", nil)
	if err != nil {
		t.Fatalf("sim.state: %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("sim.state answered %T", v)
	}
	if m["links_measured"] != true {
		t.Errorf("sim.state reports links_measured=%v after a finished warm", m["links_measured"])
	}
	// The third state is gone: nothing holds a warm, so nothing should still
	// be publishing a field for it.
	if _, ok := m["warm_held"]; ok {
		t.Error("sim.state still carries warm_held, which nothing can set")
	}
	g, ok := m["ground"].(map[string]any)
	if !ok {
		t.Fatalf("sim.state carries no ground: %v", m)
	}
	if g["state"] != state.GroundBare || g["chosen"] != true {
		t.Errorf("an offline session reports ground %v, which is not a chosen bare earth", g)
	}
}

// Bare earth with downloads on is not a decision, it is a fetch that did not
// happen, and a study over it is refused rather than answered.
func TestBareEarthWithDownloadsOnIsNotTreatedAsAChoice(t *testing.T) {
	g := state.Ground{State: state.GroundBare, Chosen: false}
	g.Note = groundNote(g)
	w := &state.World{}
	if err := StudyGround(w, "coverage.compute", g); err == nil {
		t.Fatal("a study over ground nobody fetched was answered rather than refused")
	}
	for _, want := range []string{"downloads are on", "did not finish"} {
		if !strings.Contains(g.Note, want) {
			t.Errorf("the note does not say the fetch is what is missing: %q", g.Note)
		}
	}
}
