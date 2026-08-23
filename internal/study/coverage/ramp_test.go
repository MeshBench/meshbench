package coverage

import "testing"

// The ramp's ends are the brand's, and everything between them gets warmer.
//
// The direction is the point of the test rather than the exact colours: a ramp
// that ran the other way would still interpolate, still look like a gradient,
// and would say that thin coverage is where the signal is.
func TestRampRunsFromTheNoiseFloorUp(t *testing.T) {
	floor := Ramp(0)
	if floor != (rampStops[0]) {
		t.Errorf("0 dB is %v, want the noise floor %v", floor, rampStops[0])
	}
	top := Ramp(20)
	if top != rampStops[len(rampStops)-1] {
		t.Errorf("20 dB is %v, want the top %v", top, rampStops[len(rampStops)-1])
	}
	// Out of range in either direction clamps rather than wrapping round to
	// the other end, which would paint a dead cell as the strongest one there
	// is.
	if Ramp(-30) != floor {
		t.Errorf("below the floor is %v, want %v", Ramp(-30), floor)
	}
	if Ramp(1000) != top {
		t.Errorf("above the top is %v, want %v", Ramp(1000), top)
	}

	// Red rises across the whole run: violet through to a hot orange never
	// goes back down it, so this catches a stop entered out of order.
	last := -1
	for db := 0.0; db <= 20; db += 0.5 {
		if r := int(Ramp(db).R); r < last {
			t.Fatalf("%g dB is redder going down: %d after %d", db, r, last)
		} else {
			last = r
		}
	}
}
