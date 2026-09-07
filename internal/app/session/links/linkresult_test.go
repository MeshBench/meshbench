package links

import (
	"context"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// link.pair starts a worker and answers with two labels. The cut-through and
// both margins it computes landed in the snapshot, where only a panel could
// reach them, so the question the verb exists to answer could be asked from a
// script and not read back.
func TestLinkResultAnswersBothDirections(t *testing.T) {
	st := state.New(10)
	registerLinkResult(st, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go st.Run(ctx)
	defer cancel()

	// Nothing analysed yet: refused, and the refusal says how to get one.
	if _, err := st.Do(ctx, "link.result", nil); err == nil {
		t.Fatal("link.result answered before anything had been analysed")
	} else if !strings.Contains(err.Error(), "link.pair") {
		t.Errorf("the refusal does not say how to get a result: %v", err)
	}

	// The world is written the way the analysis writes it, through a verb on
	// the store's own goroutine.
	st.Handle("test.seed", func(w *state.World, _ any) (any, error) {
		w.LinkProfile = &state.Profile{
			From: "Ben Nevis", To: "Fort William", DistanceKm: 12.5,
			AtoB: 9.5, BtoA: -2.5, Verdict: "one way only",
			Assumed: "bare earth + 3.0 dB excess",
			Edges:   make([]state.ProfileEdge, 2),
			Samples: make([]state.ProfileSample, 256),
		}
		w.Budgets = []state.Budget{
			{From: "Ben Nevis", To: "Fort William", MarginDB: 9.5},
			{From: "Fort William", To: "Ben Nevis", MarginDB: -2.5},
		}
		return nil, nil
	})
	if _, err := st.Do(ctx, "test.seed", nil); err != nil {
		t.Fatal(err)
	}

	got, err := st.Do(ctx, "link.result", nil)
	if err != nil {
		t.Fatalf("link.result refused a link that had been analysed: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("link.result answered %T", got)
	}
	// Both directions, and they must differ: a margin that does not say which
	// direction is wrong even when the arithmetic is right.
	if m["a_to_b_db"] != 9.5 || m["b_to_a_db"] != -2.5 {
		t.Errorf("the two directions came back as %v and %v", m["a_to_b_db"], m["b_to_a_db"])
	}
	dirs, ok := m["directions"].([]map[string]any)
	if !ok || len(dirs) != 2 {
		t.Fatalf("directions came back as %#v", m["directions"])
	}
	if dirs[0]["from"] == dirs[1]["from"] {
		t.Error("both directions are from the same end")
	}
	if m["km"] != 12.5 || m["edges"] != 2 || m["samples"] != 256 {
		t.Errorf("the cut-through came back as %v km, %v edges, %v samples",
			m["km"], m["edges"], m["samples"])
	}
	// The provenance travels with the numbers: a margin whose model is silent
	// reads as measured.
	if s, _ := m["assumed"].(string); !strings.Contains(s, "bare earth") {
		t.Errorf("the loss model was not reported: %q", s)
	}
}
