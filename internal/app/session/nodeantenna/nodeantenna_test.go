package nodeantenna_test

import (
	"context"
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	_ "github.com/MeshBench/meshbench/internal/app/session/nodeantenna"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// A mesh of two, placed rather than imported, because a placed node is the one
// that used to arrive with no antenna at all.
func newSim(t *testing.T) (*state.Store, context.Context) {
	t.Helper()
	st := state.New(10)
	session.Register(st, &session.Sim{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go st.Run(ctx)
	for _, n := range []struct {
		name     string
		lat, lon float64
	}{
		{"Mast", 56.75, -3.72},
		{"Valley", 56.60, -3.72}, // due south of the mast
	} {
		if _, err := st.Do(ctx, "nodes.place", map[string]any{
			"name": n.name, "lat": n.lat, "lon": n.lon, "board": "RAK4631",
		}); err != nil {
			t.Fatal(err)
		}
	}
	return st, ctx
}

func got(t *testing.T, st *state.Store, ctx context.Context, verb string, p any) map[string]any {
	t.Helper()
	out, err := st.Do(ctx, verb, p)
	if err != nil {
		t.Fatalf("%s: %v", verb, err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("%s answered %T, not an object", verb, out)
	}
	return m
}

// A placed node used to carry no antenna at all: nil pattern, zero gain in the
// engine, no overlay on the map, and a nil dereference through the coverage
// closures. It now stands under the board's own.
func TestAPlacedNodeArrivesWithAnAntenna(t *testing.T) {
	st, ctx := newSim(t)
	a := got(t, st, ctx, "node.antenna", "Mast")
	if a["pattern"] != "collinear" {
		t.Errorf("a placed node's antenna is %q, wanted a collinear", a["pattern"])
	}
	if a["polarisation"] != "vertical" {
		t.Errorf("polarisation %q, wanted vertical", a["polarisation"])
	}
	if g, _ := a["peak_dbi"].(float64); g <= 0 {
		t.Errorf("peak gain %v, wanted the board's figure", a["peak_dbi"])
	}
}

// The whole point of the issue: a yagi cannot be built by any reachable path.
// Now it can, and what comes back is what went in.
func TestAYagiCanBeBuiltAndReadBack(t *testing.T) {
	st, ctx := newSim(t)
	got(t, st, ctx, "nodes.antenna", map[string]any{
		"node": "Mast", "pattern": "yagi", "gain_dbi_peak": 12.0,
		"beamwidth_deg": 45.0, "front_to_back_db": 22.0, "bearing_deg": 180.0,
		"downtilt_deg": 3.0, "polarisation": "horizontal", "feedline_db": 1.5,
	})
	a := got(t, st, ctx, "node.antenna", "Mast")
	for _, want := range []struct {
		key string
		val any
	}{
		{"pattern", "yagi"}, {"gain_dbi_peak", 12.0}, {"beamwidth_deg", 45.0},
		{"front_to_back_db", 22.0}, {"bearing_deg", 180.0}, {"downtilt_deg", 3.0},
		{"polarisation", "horizontal"}, {"feedline_db", 1.5},
	} {
		if a[want.key] != want.val {
			t.Errorf("%s came back %v, wanted %v", want.key, a[want.key], want.val)
		}
	}
	// The other node is untouched: one node named is one node changed.
	if b := got(t, st, ctx, "node.antenna", "Valley"); b["pattern"] != "collinear" {
		t.Errorf("naming one node changed another: Valley is now %q", b["pattern"])
	}
}

// Partial on purpose. The useful call is "turn this one", not "state every
// parameter again", so what was not named has to survive.
func TestSettingOnlyTheBearingKeepsTheRest(t *testing.T) {
	st, ctx := newSim(t)
	got(t, st, ctx, "nodes.antenna", map[string]any{
		"node": "Mast", "pattern": "yagi", "gain_dbi_peak": 14.0,
	})
	got(t, st, ctx, "nodes.antenna", map[string]any{
		"node": "Mast", "bearing_deg": -90.0,
	})
	a := got(t, st, ctx, "node.antenna", "Mast")
	if a["pattern"] != "yagi" || a["gain_dbi_peak"] != 14.0 {
		t.Errorf("turning the antenna replaced it: %v at %v dBi",
			a["pattern"], a["gain_dbi_peak"])
	}
	// A person types -90 for west and means west.
	if a["bearing_deg"] != 270.0 {
		t.Errorf("bearing came back %v, wanted 270", a["bearing_deg"])
	}
}

// Nothing named means every node, which is what makes a 58-node scenario
// editable at all.
func TestNoFilterMeansEveryNode(t *testing.T) {
	st, ctx := newSim(t)
	out := got(t, st, ctx, "nodes.antenna", map[string]any{"downtilt_deg": 2.0})
	if out["nodes"] != 2 {
		t.Errorf("changed %v nodes, wanted both", out["nodes"])
	}
	for _, n := range []string{"Mast", "Valley"} {
		if a := got(t, st, ctx, "node.antenna", n); a["downtilt_deg"] != 2.0 {
			t.Errorf("%s was left at %v downtilt", n, a["downtilt_deg"])
		}
	}
}

// Aiming is the affordance somebody actually wants, and it has to say what it
// won: on an omni the answer is "nothing", and a control that reports success
// while changing nothing is one nobody trusts twice.
func TestAimingPointsAtTheOtherNodeAndSaysWhatItWon(t *testing.T) {
	st, ctx := newSim(t)
	got(t, st, ctx, "nodes.antenna", map[string]any{
		"node": "Mast", "pattern": "yagi", "gain_dbi_peak": 12.0,
		"beamwidth_deg": 40.0, "bearing_deg": 0.0,
	})
	before := got(t, st, ctx, "node.antenna", "Mast")
	out := got(t, st, ctx, "node.aim", map[string]any{"node": "Mast", "at": "Valley"})

	// Valley is due south, so the beam swings from north to about 180.
	if b, _ := out["bearing_deg"].(float64); math.Abs(b-180) > 1 {
		t.Errorf("aimed at %v degrees, wanted about 180", out["bearing_deg"])
	}
	if before["bearing_deg"] == out["bearing_deg"] {
		t.Error("aiming left the antenna where it was")
	}
	// Pointed at it, the far end is on the boresight and gets the peak.
	if g, _ := out["gain_dbi"].(float64); g < 11 {
		t.Errorf("gain towards Valley is %v dBi, wanted the yagi's own %v",
			out["gain_dbi"], 12.0)
	}
	if d, _ := out["distance_km"].(float64); d < 10 || d > 25 {
		t.Errorf("distance %v km is not the separation these two were placed at", d)
	}
}

// A typo must not cost twenty decibels in silence. CrossPolLossDB reads an
// unrecognised polarisation as orthogonal to everything, so an unchecked
// spelling would take a link off the air and nothing would say why.
func TestABadPolarisationIsRefused(t *testing.T) {
	st, ctx := newSim(t)
	if _, err := st.Do(ctx, "nodes.antenna", map[string]any{
		"node": "Mast", "polarisation": "sideways",
	}); err == nil {
		t.Fatal("a polarisation the model cannot price was accepted")
	}
	if a := got(t, st, ctx, "node.antenna", "Mast"); a["polarisation"] != "vertical" {
		t.Errorf("a refused change was applied anyway: %v", a["polarisation"])
	}
}

// The same for a pattern nothing can build. Substituting an omni for a beam
// somebody asked for would change every answer and say nothing.
func TestAnUnknownPatternIsRefused(t *testing.T) {
	st, ctx := newSim(t)
	if _, err := st.Do(ctx, "nodes.antenna", map[string]any{
		"node": "Mast", "pattern": "parabolic",
	}); err == nil {
		t.Fatal("a pattern the model does not have was accepted")
	}
}

func TestNamingANodeThatIsNotThereIsRefused(t *testing.T) {
	st, ctx := newSim(t)
	for _, c := range []struct {
		verb string
		p    any
	}{
		{"node.antenna", "Nowhere"},
		{"nodes.antenna", map[string]any{"node": "Nowhere", "bearing_deg": 10.0}},
		{"node.aim", map[string]any{"node": "Mast", "at": "Nowhere"}},
		{"node.aim", map[string]any{"node": "Mast", "at": "Mast"}},
	} {
		if _, err := st.Do(ctx, c.verb, c.p); err == nil {
			t.Errorf("%s accepted %v", c.verb, c.p)
		}
	}
}

// The snapshot has to carry the change, or the map draws the antenna that used
// to be there and the aiming control looks like it does nothing.
func TestTheSnapshotFollowsTheAntenna(t *testing.T) {
	st, ctx := newSim(t)
	got(t, st, ctx, "nodes.antenna", map[string]any{
		"node": "Mast", "pattern": "yagi", "gain_dbi_peak": 12.0,
		"beamwidth_deg": 40.0, "bearing_deg": 90.0,
	})
	var mast state.Node
	for _, n := range st.Snapshot().Nodes {
		if n.Name == "Mast" {
			mast = n
		}
	}
	if mast.Antenna.Type != "yagi" || mast.Antenna.BearingDeg != 90 {
		t.Fatalf("the snapshot says %q at %v degrees",
			mast.Antenna.Type, mast.Antenna.BearingDeg)
	}
	if len(mast.Pattern) == 0 {
		t.Fatal("the snapshot carries no sampled pattern, so the map draws nothing")
	}
	// The sampled shape has to point east too, or the overlay and the setting
	// disagree on screen. Samples run every ten degrees from north.
	east, north := mast.Pattern[9], mast.Pattern[0]
	if east <= north {
		t.Errorf("the drawn pattern peaks at %.1f dBi north and %.1f east, "+
			"so it is not aimed where the node says it is", north, east)
	}
}
