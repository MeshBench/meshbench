package antenna_test

import (
	"encoding/json"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
)

// A saved scenario must come back with the same antennas it left with. Before
// Mounted knew JSON, the pattern silently unmarshalled to nil and every loaded
// node had zero gain — which looked like a propagation problem, not a
// serialisation one.
func TestMountedRoundTripsEveryPattern(t *testing.T) {
	cases := []antenna.Mounted{
		{Pattern: antenna.Isotropic{}, Polarisation: "vertical"},
		{Pattern: antenna.Dipole{}, FeedlineDB: 1.5},
		{Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical", FeedlineDB: 0.5},
		{Pattern: antenna.Yagi{GainDBiPeak: 11, BeamwidthDeg: 40, FrontToBackDB: 18},
			BearingDeg: 210, DowntiltDeg: 3},
	}
	for _, in := range cases {
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("%s: %v", in.Pattern.Name(), err)
		}
		var out antenna.Mounted
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("%s: %v", in.Pattern.Name(), err)
		}
		if out.Pattern == nil {
			t.Fatalf("%s came back with no pattern", in.Pattern.Name())
		}
		// The behavioural check, not a struct comparison: what matters is that
		// the reloaded antenna answers with the same gain.
		for _, dir := range [][2]float64{{0, 0}, {90, 5}, {180, -2}} {
			want := in.GainTowardsDBi(dir[0], dir[1])
			got := out.GainTowardsDBi(dir[0], dir[1])
			if want != got {
				t.Errorf("%s at az %.0f el %.0f: %.2f dBi in, %.2f dBi out",
					in.Pattern.Name(), dir[0], dir[1], want, got)
			}
		}
	}
}

// An unknown pattern type must refuse to load, not become isotropic. A saved
// file from a newer version naming a pattern this build lacks is a fact worth
// stopping for; a mast quietly downgraded to 0 dBi is not.
func TestUnknownPatternIsAnError(t *testing.T) {
	var m antenna.Mounted
	err := json.Unmarshal([]byte(`{"pattern":{"type":"phased-array"}}`), &m)
	if err == nil {
		t.Fatal("a pattern type this build does not know loaded anyway")
	}
}
