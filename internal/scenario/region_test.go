package scenario

import "testing"

func box(name string, s, n, w, e float64) Boundary {
	return Boundary{Name: name, Source: "drawn", Rings: []Ring{{
		{s, w}, {s, e}, {n, e}, {n, w},
	}}}
}

func TestUnionOfSeveralBoundaries(t *testing.T) {
	r := Region{Boundaries: []Boundary{
		box("Highland", 57.0, 58.0, -5.0, -3.0),
		box("Moray", 57.3, 57.8, -3.0, -2.5),
	}}
	for _, tc := range []struct {
		name string
		p    LatLon
		want bool
	}{
		{"in the first", LatLon{57.5, -4.0}, true},
		{"in the second", LatLon{57.5, -2.7}, true},
		{"in neither", LatLon{56.0, -4.0}, false},
	} {
		if got := r.Contains(tc.p); got != tc.want {
			t.Errorf("%s: Contains = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The subtle failure ADR-0019 names: a repeater just outside the boundary still
// interferes with, and relays to, nodes inside it. Dropping it silently produces
// a mesh that behaves better than reality.
func TestMarginKeepsOutsideParticipantsInTheSimulation(t *testing.T) {
	r := Region{Boundaries: []Boundary{box("study", 57.0, 57.5, -4.0, -3.5)}, MarginKm: 30}

	just := LatLon{57.6, -3.75} // ~11 km north of the boundary
	far := LatLon{58.5, -3.75}  // ~110 km north

	if r.Contains(just) {
		t.Fatal("test setup wrong: the point should be outside the boundary")
	}
	if !r.Participates(just) {
		t.Error("a node 11 km outside must still participate in RF — otherwise the " +
			"simulated mesh behaves better than reality")
	}
	if r.Participates(far) {
		t.Error("a node 110 km outside is beyond plausible LoRa reach and should not participate")
	}
}

// Contains and Participates must stay distinct: results are reported for the
// study area, RF is computed over the wider set.
func TestContainsAndParticipatesDiffer(t *testing.T) {
	r := Region{Boundaries: []Boundary{box("study", 57.0, 57.5, -4.0, -3.5)}, MarginKm: 30}
	p := LatLon{57.6, -3.75}
	if r.Contains(p) == r.Participates(p) {
		t.Error("a point in the margin must participate without being contained")
	}
}

func TestBoundsIncludeTheMargin(t *testing.T) {
	r := Region{Boundaries: []Boundary{box("study", 57.0, 57.5, -4.0, -3.5)}, MarginKm: 30}
	s, n, w, e := r.Bounds()
	if s >= 57.0 || n <= 57.5 || w >= -4.0 || e <= -3.5 {
		t.Errorf("bounds %.3f..%.3f, %.3f..%.3f do not extend past the boundary", s, n, w, e)
	}
	// Roughly 30 km ≈ 0.27 degrees of latitude.
	if d := 57.0 - s; d < 0.2 || d > 0.35 {
		t.Errorf("latitude margin %.3f deg is not ~0.27 for 30 km", d)
	}
}

func TestDefaultMarginApplies(t *testing.T) {
	r := Region{Boundaries: []Boundary{box("study", 57.0, 57.5, -4.0, -3.5)}} // no margin set
	if !r.Participates(LatLon{57.6, -3.75}) {
		t.Error("a zero margin should fall back to the default, not to nothing")
	}
}
