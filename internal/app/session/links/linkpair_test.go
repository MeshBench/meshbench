package links_test

import (
	"context"
	"github.com/MeshBench/meshbench/internal/app/session"
	_ "github.com/MeshBench/meshbench/internal/app/session/links"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// link.pair exists because the interface's link tool picked a pair and the
// session substituted the first node's strongest link - the chosen second
// node was never consulted, and a far-apart pair produced no picture at all.
// These hold the verb to the pair it was given.

func pairStore(t *testing.T) (*state.Store, context.Context, context.CancelFunc) {
	t.Helper()
	st := state.New(10)
	session.Register(st, &session.Sim{})
	ctx, cancel := context.WithCancel(context.Background())
	go st.Run(ctx)
	return st, ctx, cancel
}

// waitProfile polls until the worker has delivered a profile for from→to.
func waitProfile(t *testing.T, st *state.Store, from, to string) *state.Profile {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s := st.Snapshot(); s != nil && s.LinkProfile != nil &&
			s.LinkProfile.From == from && s.LinkProfile.To == to {
			return s.LinkProfile
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no profile for %s → %s arrived", from, to)
	return nil
}

func TestLinkPairHonoursThePair(t *testing.T) {
	st, ctx, cancel := pairStore(t)
	defer cancel()
	if _, err := st.Do(ctx, "project.open", "../../../fixtures/fixture-fife-strict.json"); err != nil {
		t.Skip("no fixture:", err)
	}
	s := st.Snapshot()
	if len(s.Nodes) < 2 {
		t.Skip("fixture has fewer than two nodes")
	}
	// Deliberately not neighbours: the far ends of the list, which is the
	// kind of pair the engine's link table drops.
	a, b := s.Nodes[0].Name, s.Nodes[len(s.Nodes)-1].Name

	if _, err := st.Do(ctx, "link.pair", map[string]any{"a": a, "b": b}); err != nil {
		t.Fatal(err)
	}
	p := waitProfile(t, st, a, b)
	if len(p.Samples) < 8 {
		t.Fatalf("profile has %d samples; there is no picture in that", len(p.Samples))
	}
	if p.Assumed == "" {
		t.Fatal("the margins carry no provenance; a silent model reads as measured")
	}
	if p.AtoB == 0 && p.BtoA == 0 {
		t.Fatal("both margins are exactly zero, which is no margin at all")
	}
	got := st.Snapshot()
	if len(got.Budgets) != 2 {
		t.Fatalf("budgets carry %d directions, want both", len(got.Budgets))
	}
	if got.Budgets[0].From != a || got.Budgets[0].To != b ||
		got.Budgets[1].From != b || got.Budgets[1].To != a {
		t.Fatalf("budgets are for %s→%s and %s→%s, not the pair asked about",
			got.Budgets[0].From, got.Budgets[0].To,
			got.Budgets[1].From, got.Budgets[1].To)
	}
}

func TestLinkPairTakesTwoPlaces(t *testing.T) {
	st, ctx, cancel := pairStore(t)
	defer cancel()
	res, err := st.Do(ctx, "link.pair", map[string]any{
		"a": map[string]any{"lat": 56.2, "lon": -3.3},
		"b": map[string]any{"lat": 56.25, "lon": -3.2, "height_m": 8.0},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]any)
	from, to := m["from"].(string), m["to"].(string)
	p := waitProfile(t, st, from, to)
	// No scenario is loaded, so the band was assumed - and must be said.
	if !strings.Contains(p.Assumed, "assumed") {
		t.Fatalf("Assumed is %q; an invented frequency has to be admitted to", p.Assumed)
	}
	if p.DistanceKm < 5 || p.DistanceKm > 10 {
		t.Fatalf("distance %f km for a ~7 km pair", p.DistanceKm)
	}
}

func TestLinkPairRefusals(t *testing.T) {
	st, ctx, cancel := pairStore(t)
	defer cancel()
	cases := []map[string]any{
		{"a": "nobody"},
		{"a": map[string]any{"lat": 56.0}},
		{"a": map[string]any{"lat": 56.0, "lon": -3.0},
			"b": map[string]any{"lat": 56.0, "lon": -3.0}},
	}
	for _, p := range cases {
		if _, err := st.Do(ctx, "link.pair", p); err == nil {
			t.Errorf("link.pair %v was accepted; it should refuse and say why", p)
		}
	}
}
