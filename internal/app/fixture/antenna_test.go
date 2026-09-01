package fixture_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/fixture"
	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A fixture carries the antenna, aim and all.
//
// The nodes go to disk as scenario.Node, so this rides on the antenna package's
// own type tag rather than on a second format - but nothing had ever proved it,
// because until the antenna verbs landed nothing could set a bearing to lose.
// A beam that reloads as an omni pointing north is the failure this guards, and
// it would look exactly like a propagation problem.
func TestAFixtureKeepsTheAntennaItWasSaidWith(t *testing.T) {
	want := scenario.Node{
		Name: "Mast", Kind: scenario.SimpleRepeater,
		Position:   scenario.LatLon{Lat: 56.75, Lon: -3.72},
		HeightAGLm: 10, TxPowerDBm: 22,
		Radio: scenario.RadioConfig{
			CentreHz: 869.618e6, BandwidthHz: 62500, SpreadFactor: 8, CodingRate: 4,
		},
		Antenna: antenna.Mounted{
			Pattern: antenna.Yagi{
				GainDBiPeak: 12, BeamwidthDeg: 45, FrontToBackDB: 22,
			},
			BearingDeg: 217, DowntiltDeg: 3,
			Polarisation: antenna.Horizontal, FeedlineDB: 1.5,
		},
	}
	if err := want.Validate(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "beam.json")
	blob, err := json.Marshal(fixture.Fixture{
		Name: "beam", Saved: time.Now(), Nodes: []scenario.Node{want},
		Seed: 1, FreqMHz: 869.618,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatal(err)
	}

	back, err := fixture.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got := back.Nodes[0].Antenna
	beam, ok := got.Pattern.(antenna.Yagi)
	if !ok {
		t.Fatalf("a yagi came back as %T, so a saved beam is not a beam", got.Pattern)
	}
	if beam != (antenna.Yagi{GainDBiPeak: 12, BeamwidthDeg: 45, FrontToBackDB: 22}) {
		t.Errorf("the beam came back as %+v", beam)
	}
	if got.BearingDeg != 217 || got.DowntiltDeg != 3 {
		t.Errorf("aimed at %v degrees and tilted %v, not where it was left",
			got.BearingDeg, got.DowntiltDeg)
	}
	if got.Polarisation != antenna.Horizontal || got.FeedlineDB != 1.5 {
		t.Errorf("polarisation %q and %v dB of feedline", got.Polarisation, got.FeedlineDB)
	}
	// And the gain it claims is the gain it claimed, which is the only part of
	// this a user ever sees.
	if a, b := want.Antenna.GainAlongDBi(217), got.GainAlongDBi(217); a != b {
		t.Errorf("boresight gain went from %.2f dBi to %.2f across a save", a, b)
	}
}
