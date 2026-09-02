package inventory

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// listing is nodes.list over a network of two, with the store's goroutine
// running behind it.
func listing(t *testing.T) []map[string]any {
	t.Helper()
	st := state.New(10)
	registerInventory(st, &session.Sim{})
	st.Handle("test.nodes", func(w *state.World, _ any) (any, error) {
		w.Nodes = []state.Node{
			{Name: "West Lomond", Kind: "simple_repeater", Lat: 56.25, Lon: -3.29},
			{Name: "Dunfermline", Kind: "companion_radio", Lat: 56.07, Lon: -3.46},
		}
		return nil, nil
	})
	go st.Run(t.Context())
	if _, err := st.Do(t.Context(), "test.nodes", nil); err != nil {
		t.Fatal(err)
	}
	got, err := st.Do(t.Context(), "nodes.list", nil)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("nodes.list answered a %T", got)
	}
	rows, ok := m["nodes"].([]map[string]any)
	if !ok {
		t.Fatalf("nodes is a %T", m["nodes"])
	}
	if len(rows) != 2 {
		t.Fatalf("%d rows, want 2", len(rows))
	}
	return rows
}

// nodes.list offers no counter that nothing writes.
//
// It published a "sent" and a "heard" for every node and nothing in the tree
// ever assigned either, so both were nought in every reply anybody ever
// received - on a mesh carrying thousands of packets as readily as on a mesh
// that had never played. A caller cannot tell that number from a real one, so
// it does not read as a missing field; it reads as a silent network, which is
// a wrong answer to the question the verb exists to answer.
//
// The counts are real on nodes.stats, measured from the engine's own
// scoreboard. Absent here is a question the caller knows to ask elsewhere.
//
// Asserted as absence rather than as correctness on purpose: a test that only
// checked the remaining fields would have passed against the old reply too,
// which is exactly the failure mode this whole change is about.
func TestNodeListOffersNoCounterNothingWrites(t *testing.T) {
	for _, row := range listing(t) {
		for _, key := range []string{"sent", "heard"} {
			if v, present := row[key]; present {
				t.Errorf("%v carries %q = %v; nothing writes it, so it is nought "+
					"for every node in every reply - nodes.stats has the counts",
					row["name"], key, v)
			}
		}
	}
}

// What it does still carry, so removing two fields did not quietly cost a
// third.
func TestNodeListStillCarriesWhatANetworkIs(t *testing.T) {
	row := listing(t)[0]
	for _, key := range []string{
		"name", "kind", "lat", "lon", "height_m", "tx_dbm", "regions",
		"firmware", "board", "firmware_board", "selected",
	} {
		if _, present := row[key]; !present {
			t.Errorf("nodes.list no longer carries %q", key)
		}
	}
}
