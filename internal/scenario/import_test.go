package scenario_test

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/provider"
	"github.com/MeshBench/meshbench/internal/scenario"
)

func importOpts() scenario.ImportOptions {
	return scenario.ImportOptions{
		DefaultBoard: "RAK4631",
		Radio: scenario.RadioConfig{
			CentreHz: 869.525e6, BandwidthHz: 250e3, SpreadFactor: 10, CodingRate: 1,
		},
		MaxUncertaintyKm: 1,
	}
}

// A node with no position cannot be simulated. Placing it at (0,0) puts it in
// the Atlantic, where it quietly fails to reach anything and looks like a
// coverage result rather than missing data.
func TestPositionlessNodesAreCountedNotPlaced(t *testing.T) {
	res, err := scenario.Import([]provider.NodeRecord{
		{Name: "known", HasPosition: true, Lat: 56.7, Lon: -3.9},
		{Name: "unplaced"},
	}, importOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Nodes) != 1 {
		t.Fatalf("imported %d nodes, want 1", len(res.Nodes))
	}
	if res.SkippedNoPosition != 1 {
		t.Errorf("skipped-no-position is %d, want 1", res.SkippedNoPosition)
	}
	if !strings.Contains(res.Describe(), "no position at all") {
		t.Errorf("the summary hides the missing records:\n%s", res.Describe())
	}
}

// An uncertain node is imported and marked, not dropped and not trusted.
// Dropping it loses the fact that it exists; trusting it produces answers to a
// decibel about something known to a kilometre.
func TestUncertainNodesAreKeptAndFlagged(t *testing.T) {
	res, err := scenario.Import([]provider.NodeRecord{
		{Name: "surveyed", HasPosition: true, Lat: 56.7, Lon: -3.9, UncertaintyKm: 0.05},
		{Name: "inferred", HasPosition: true, Lat: 56.8, Lon: -3.8, UncertaintyKm: 5},
	}, importOpts())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Nodes) != 2 {
		t.Fatalf("imported %d nodes, want 2", len(res.Nodes))
	}
	byName := map[string]scenario.Imported{}
	for _, n := range res.Nodes {
		byName[n.Node.Name] = n
	}
	if byName["surveyed"].Uncertain {
		t.Error("a 50 m position was marked uncertain")
	}
	if !byName["inferred"].Uncertain {
		t.Error("a 5 km position was not marked uncertain")
	}
	if len(byName["inferred"].Warnings) == 0 {
		t.Error("the uncertain node carries no warning")
	}
	// The uncertainty must survive onto the node itself, not just the report.
	if byName["inferred"].Node.UncertaintyKm != 5 {
		t.Errorf("node uncertainty is %.1f km", byName["inferred"].Node.UncertaintyKm)
	}
}

// A repeater just outside the study area still relays to and interferes with
// nodes inside. Dropping it produces a mesh that behaves better than reality.
func TestNodesOutsideButInRangeAreKeptAsParticipants(t *testing.T) {
	bs, err := scenario.ParseGeoJSON([]byte(`{"type":"Polygon","coordinates":[
		[[-4.0,56.6],[-3.6,56.6],[-3.6,56.9],[-4.0,56.9],[-4.0,56.6]]]}`), "")
	if err != nil {
		t.Fatal(err)
	}
	o := importOpts()
	o.Region = &scenario.Region{Boundaries: bs, MarginKm: scenario.DefaultMarginKm}

	res, err := scenario.Import([]provider.NodeRecord{
		{Name: "inside", HasPosition: true, Lat: 56.75, Lon: -3.8},
		{Name: "just-outside", HasPosition: true, Lat: 56.55, Lon: -3.8},
		{Name: "far-away", HasPosition: true, Lat: 51.5, Lon: -0.1},
	}, o)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]scenario.Imported{}
	for _, n := range res.Nodes {
		byName[n.Node.Name] = n
	}
	if _, ok := byName["inside"]; !ok {
		t.Error("a node inside the boundary was dropped")
	}
	p, ok := byName["just-outside"]
	if !ok {
		t.Fatal("a node just outside the boundary was dropped; it still interferes")
	}
	if !p.Participant {
		t.Error("the nearby outside node was not marked as a participant")
	}
	if _, ok := byName["far-away"]; ok {
		t.Error("a node in London was imported into a Perthshire scenario")
	}
	if res.SkippedOutside != 1 {
		t.Errorf("skipped-outside is %d, want 1", res.SkippedOutside)
	}
}

// After terrain, antenna height is the largest factor in whether a path works.
// Applying a default silently is how a scenario ends up full of ten-metre
// handhelds.
func TestAssumedHeightIsFlagged(t *testing.T) {
	res, err := scenario.Import([]provider.NodeRecord{
		{Name: "no-height", HasPosition: true, Lat: 56.7, Lon: -3.9},
		{Name: "known-height", HasPosition: true, Lat: 56.8, Lon: -3.8, HeightAGLm: 25},
	}, importOpts())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]scenario.Imported{}
	for _, n := range res.Nodes {
		byName[n.Node.Name] = n
	}
	if len(byName["no-height"].Warnings) == 0 {
		t.Error("an assumed antenna height was applied without a warning")
	}
	if byName["no-height"].Node.HeightAGLm <= 0 {
		t.Error("no height was applied at all, so the node cannot be simulated")
	}
	for _, w := range byName["known-height"].Warnings {
		if strings.Contains(w, "height") {
			t.Errorf("a recorded height was flagged as assumed: %q", w)
		}
	}
}

// There is no neutral default board. Picking one silently gives every imported
// node someone else's transmit power.
func TestImportRefusesWithoutABoard(t *testing.T) {
	o := importOpts()
	o.DefaultBoard = ""
	_, err := scenario.Import(nil, o)
	if err == nil {
		t.Fatal("an import with no board was accepted")
	}
	if !strings.Contains(err.Error(), "neutral") {
		t.Errorf("the error should say why: %v", err)
	}

	o = importOpts()
	o.Radio = scenario.RadioConfig{}
	if _, err := scenario.Import(nil, o); err == nil {
		t.Fatal("an import with no radio configuration was accepted")
	}
}

// Two nodes sharing a name are indistinguishable in every ledger entry and
// every export.
func TestDuplicateNamesAreMadeUnique(t *testing.T) {
	res, err := scenario.Import([]provider.NodeRecord{
		{Name: "repeater", PublicKey: "aaaaaaaabbbb", HasPosition: true, Lat: 56.7, Lon: -3.9},
		{Name: "repeater", PublicKey: "ccccccccdddd", HasPosition: true, Lat: 56.8, Lon: -3.8},
		{PublicKey: "eeeeeeeeffff", HasPosition: true, Lat: 56.6, Lon: -3.7},
	}, importOpts())
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, n := range res.Nodes {
		if names[n.Node.Name] {
			t.Errorf("duplicate name %q survived the import", n.Node.Name)
		}
		names[n.Node.Name] = true
		if n.Node.Name == "" {
			t.Error("a node was imported with no name")
		}
	}
	if len(res.Nodes) != 3 {
		t.Errorf("imported %d nodes, want 3", len(res.Nodes))
	}
}

// An unknown role becomes a repeater rather than stopping the import: it
// transmits, so it is accounted for in everyone else's interference rather than
// being silently absent from it.
func TestUnknownRolesBecomeRepeaters(t *testing.T) {
	res, err := scenario.Import([]provider.NodeRecord{
		{Name: "odd", Kind: "weather-station", HasPosition: true, Lat: 56.7, Lon: -3.9},
		{Name: "phone", Kind: "companion", HasPosition: true, Lat: 56.8, Lon: -3.8},
	}, importOpts())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]scenario.Imported{}
	for _, n := range res.Nodes {
		byName[n.Node.Name] = n
	}
	if byName["odd"].Node.Kind != scenario.SimpleRepeater {
		t.Errorf("an unknown role became %s", byName["odd"].Node.Kind)
	}
	if byName["phone"].Node.Kind != scenario.Companion {
		t.Errorf("a companion became %s", byName["phone"].Node.Kind)
	}
}

// Everything imported must be a valid scenario node, or the import has merely
// deferred the failure to whatever runs next.
func TestEverythingImportedValidates(t *testing.T) {
	res, err := scenario.Import([]provider.NodeRecord{
		{Name: "a", HasPosition: true, Lat: 56.7, Lon: -3.9},
		{Name: "b", Kind: "observer", HasPosition: true, Lat: 56.8, Lon: -3.8},
		{Name: "c", Kind: "companion", HasPosition: true, Lat: 56.6, Lon: -3.7, HeightAGLm: 2},
	}, importOpts())
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range res.Nodes {
		if err := n.Node.Validate(); err != nil {
			t.Errorf("%s: %v", n.Node.Name, err)
		}
	}
	// An observer must not have been given a transmit power on the way in.
	for _, n := range res.Nodes {
		if n.Node.Kind == scenario.SDRObserver && n.Node.TxPowerDBm != 0 {
			t.Errorf("observer %s was given %.0f dBm", n.Node.Name, n.Node.TxPowerDBm)
		}
	}
}
