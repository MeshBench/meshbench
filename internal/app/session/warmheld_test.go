// What a warm that stopped to ask reports about itself.
//
// The bug this pins was silent in the worst way available: the held warm marked
// the session warmed and finished its own job row, so a script that opened a
// fixture, waited for idle and read a result got one. Not an error, not an empty
// answer, not a timeout - a plausible number over free space, from a session
// that said it was ready.
package session

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// heldSession opens a network on a machine nobody has asked about terrain, and
// waits for the warm to reach a decision either way.
func heldSession(t *testing.T) (*Sim, *state.Store, context.Context) {
	t.Helper()
	s, st, ctx := consentSim(t)
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
		if measured, _ := s.linksMeasured(); measured {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the released warm never reported a measured matrix")
}

// The measurement did not happen, so nothing may say it did.
func TestAHeldWarmDoesNotReportTheLinksAsMeasured(t *testing.T) {
	s, st, ctx := heldSession(t)
	measured, held := s.linksMeasured()
	if measured {
		t.Error("a warm that measured no link reports the matrix as measured")
	}
	if !held {
		t.Error("a warm held for permission does not say it is held")
	}
	// Still not something to wait behind: a held warm that reported itself as
	// running would block every run on a measurement nobody is doing.
	if s.warming() {
		t.Error("a held warm reports itself as still running")
	}
	if links := st.Snapshot().Links; len(links) != 0 {
		t.Errorf("%d links came out of a warm that fetched no ground", len(links))
	}

	// And the polled verb says the same, because a script never reads a field.
	v, err := st.Do(ctx, "sim.state", nil)
	if err != nil {
		t.Fatalf("sim.state: %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("sim.state answered %T", v)
	}
	if m["links_measured"] != false || m["warm_held"] != true {
		t.Errorf("sim.state reports links_measured=%v warm_held=%v over a held warm",
			m["links_measured"], m["warm_held"])
	}
	g, ok := m["ground"].(map[string]any)
	if !ok {
		t.Fatalf("sim.state carries no ground: %v", m)
	}
	if g["state"] != state.GroundBare || g["chosen"] != false {
		t.Errorf("a held session reports ground %v, which is not an unchosen bare earth", g)
	}
	// The job row is the other thing a script reads, and it has to say the
	// measurement did not happen rather than that it did.
	for _, j := range st.Snapshot().Jobs {
		if j.ID != "links" {
			continue
		}
		if !j.Failed {
			t.Errorf("the held link job is not marked failed: %+v", j)
		}
		if j.Done != 0 {
			t.Errorf("the held link job claims %d of %d measured", j.Done, j.Total)
		}
	}
}

// Refusing is an answer, and an answer releases the warm. What comes out of it
// is a measured matrix over bare earth that says so, which is the legitimate
// offline run the consent gate exists to make possible.
//
// The refusal rather than the grant, because granting downloads a country: this
// is the half of the pair that touches no network.
func TestARefusedMachineStillMeasuresItsLinksAndSaysWhatOver(t *testing.T) {
	s, st, ctx := heldSession(t)
	if _, held := s.linksMeasured(); !held {
		t.Fatal("the warm was not held to begin with")
	}
	v, err := st.Do(ctx, "terrain.allow", map[string]any{"on": false})
	if err != nil {
		t.Fatalf("terrain.allow: %v", err)
	}
	m, _ := v.(map[string]any)
	if m["warming"] != true {
		t.Fatalf("refusing the download did not restart the held warm: %v", m)
	}
	// Waited on the measurement itself rather than on a finished job row: the
	// held warm's own row is already there and already finished, so a wait for
	// one of those returns before the released warm has started.
	waitForMeasured(t, s)
	if _, held := s.linksMeasured(); held {
		t.Error("the warm is still held after being answered")
	}
	// A choice is what stops a bare-earth run being a silent fallback. It never
	// stops being announced.
	g := st.Snapshot().Ground
	if !g.Bare() || !g.Chosen {
		t.Errorf("a refused machine reports ground %+v", g)
	}
	if !strings.Contains(g.Note, "free space") {
		t.Errorf("a chosen bare-earth run does not say what it means: %q", g.Note)
	}
}
