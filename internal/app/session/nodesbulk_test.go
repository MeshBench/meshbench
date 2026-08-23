package session

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// Trimming a network, and saying what a node is.
//
// nodes.delete took one name, so cutting a fifty-eight node fixture down to
// two was fifty-six calls, each rebuilding the scenario and starting a warm
// the next one cancelled. Correct, and minutes of it.

func bulkSession(t *testing.T, names ...string) (*state.Store, context.Context) {
	t.Helper()
	st, _ := Boot(Options{NoPrefs: true, Headless: true})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go st.Run(ctx)
	if _, err := st.Do(ctx, "project.new", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	for i, n := range names {
		if _, err := st.Do(ctx, "nodes.place", map[string]any{
			"name": n, "kind": "simple-repeater",
			"lat": 56.0 + float64(i)/100, "lon": -3.0,
		}); err != nil {
			t.Fatalf("placing %s: %v", n, err)
		}
	}
	return st, ctx
}

func nodeNames(t *testing.T, st *state.Store, ctx context.Context) []string {
	t.Helper()
	got, err := st.Do(ctx, "nodes.list", nil)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := got.(map[string]any)
	rows, _ := m["nodes"].([]map[string]any)
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r["name"].(string))
	}
	return out
}

func TestKeepRemovesTheComplementInOneRebuild(t *testing.T) {
	st, ctx := bulkSession(t, "A", "B", "C", "D", "E")
	if _, err := st.Do(ctx, "nodes.keep", []string{"B", "D"}); err != nil {
		t.Fatal(err)
	}
	got := nodeNames(t, st, ctx)
	if len(got) != 2 || got[0] != "B" || got[1] != "D" {
		t.Fatalf("kept %v, want B and D", got)
	}
}

func TestDeleteManyTakesASet(t *testing.T) {
	st, ctx := bulkSession(t, "A", "B", "C")
	if _, err := st.Do(ctx, "nodes.delete_many", []string{"A", "C"}); err != nil {
		t.Fatal(err)
	}
	if got := nodeNames(t, st, ctx); len(got) != 1 || got[0] != "B" {
		t.Fatalf("after deleting A and C: %v", got)
	}
}

// Half a deletion is worse than none: a script that asked for three and got
// two has a scenario nobody described.
func TestABadNameDeletesNothing(t *testing.T) {
	st, ctx := bulkSession(t, "A", "B", "C")
	_, err := st.Do(ctx, "nodes.delete_many", []string{"A", "Nowhere"})
	if err == nil {
		t.Fatal("deleting a name that does not exist succeeded")
	}
	if got := control.CodeOf(err); got != control.NotFound {
		t.Errorf("code %q, want %q", got, control.NotFound)
	}
	if got := nodeNames(t, st, ctx); len(got) != 3 {
		t.Fatalf("a refused delete removed something anyway: %v", got)
	}
	// And it names the one it could not find, not just that one was missing.
	if !contains(err.Error(), "Nowhere") {
		t.Errorf("the refusal does not name it: %v", err)
	}
}

// "Keep everything" and "delete nothing" are both coherent, and a script
// should be able to say either without guarding the call.
func TestAnEmptySetIsNotAnError(t *testing.T) {
	st, ctx := bulkSession(t, "A", "B")
	if _, err := st.Do(ctx, "nodes.delete_many", []string{}); err != nil {
		t.Fatalf("deleting nothing: %v", err)
	}
	if got := nodeNames(t, st, ctx); len(got) != 2 {
		t.Fatalf("deleting nothing removed something: %v", got)
	}
}

func TestKeepRefusesANameThatIsNotThere(t *testing.T) {
	st, ctx := bulkSession(t, "A", "B")
	_, err := st.Do(ctx, "nodes.keep", []string{"A", "Nowhere"})
	if err == nil {
		t.Fatal("keeping a node that does not exist succeeded")
	}
	// Nothing removed: a keep that silently dropped the unknown name would
	// have deleted B on the strength of a typo.
	if got := nodeNames(t, st, ctx); len(got) != 2 {
		t.Fatalf("a refused keep removed something: %v", got)
	}
}

func TestPlacingANodeOnABoard(t *testing.T) {
	st, ctx := bulkSession(t)
	got, err := st.Do(ctx, "nodes.place", map[string]any{
		"name": "Deck", "kind": "companion",
		"lat": 56.19, "lon": -3.17, "board": "LilyGo_TDeck",
	})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := got.(map[string]any)
	if m["board"] != "LilyGo_TDeck" {
		t.Fatalf("placed with board %v", m["board"])
	}
}

// A name that is nearly right is the common case, and a board is physics -
// the transmit ceiling, the noise figure, the battery - so a wrong one must
// refuse rather than fall back to something plausible.
func TestABoardNobodyHasRefuses(t *testing.T) {
	st, ctx := bulkSession(t)
	_, err := st.Do(ctx, "nodes.place", map[string]any{
		"name": "Deck", "lat": 56.0, "lon": -3.0, "board": "LilyGo T-Deck Pro Max",
	})
	if err == nil {
		t.Fatal("a board nobody has was accepted")
	}
	if got := control.CodeOf(err); got != control.BadParams {
		t.Errorf("code %q, want %q (%v)", got, control.BadParams, err)
	}
}

func TestChangingWhatANodeIs(t *testing.T) {
	st, ctx := bulkSession(t, "Deck")
	if _, err := st.Do(ctx, "node.set_board", map[string]any{
		"node": "Deck", "board": "Heltec_v3"}); err != nil {
		t.Fatal(err)
	}
	got, err := st.Do(ctx, "node.set_board", map[string]any{
		"node": "Deck", "board": "LilyGo_TDeck"})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := got.(map[string]any)
	if m["board"] != "LilyGo_TDeck" {
		t.Fatalf("set_board reported %v", m["board"])
	}

	if _, err := st.Do(ctx, "node.set_board", map[string]any{
		"node": "Nowhere", "board": "Heltec_v3"}); err == nil {
		t.Error("setting a board on a node that does not exist succeeded")
	}
}
