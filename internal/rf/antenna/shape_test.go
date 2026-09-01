package antenna_test

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
)

// Shape is what the JSON, the verbs and the form all speak, so a pattern that
// describes itself one way and rebuilds another is the bug it exists to stop.
func TestEverySortDescribesItselfBackIntoItself(t *testing.T) {
	for _, want := range []antenna.Pattern{
		antenna.Isotropic{},
		antenna.Dipole{},
		antenna.Collinear{GainDBiPeak: 6.15},
		antenna.Yagi{GainDBiPeak: 12, BeamwidthDeg: 45, FrontToBackDB: 22},
	} {
		shape, err := antenna.ShapeOf(want)
		if err != nil {
			t.Fatalf("%s: %v", want.Name(), err)
		}
		got, err := shape.Pattern()
		if err != nil {
			t.Fatalf("%s: %v", want.Name(), err)
		}
		if got != want {
			t.Errorf("%s went out as %+v and came back as %+v", want.Name(), want, got)
		}
	}
}

// Every sort Types offers can be built, or the dropdown offers a choice that
// refuses when it is made.
func TestEverySortOfferedCanBeBuilt(t *testing.T) {
	for _, name := range antenna.Types() {
		p, err := (antenna.Shape{Type: name}).Pattern()
		if err != nil {
			t.Errorf("%q is offered and cannot be built: %v", name, err)
			continue
		}
		if p.Name() == "" {
			t.Errorf("%q builds a pattern that will not name itself", name)
		}
	}
}

// An unknown sort is an error rather than a plausible-looking omni: silently
// substituting one would change every answer and say nothing.
func TestAnUnknownSortIsRefused(t *testing.T) {
	if _, err := (antenna.Shape{Type: "parabolic"}).Pattern(); err == nil {
		t.Fatal("a sort the package does not have was built anyway")
	}
}

// Polarisation costs decibels only in relation to something else, which is why
// the pair reader exists and CrossPolLossDB alone had no caller.
func TestPolarisationIsPricedAsAPairAndUnstatedIsFree(t *testing.T) {
	v := antenna.Mounted{Pattern: antenna.Dipole{}, Polarisation: antenna.Vertical}
	h := antenna.Mounted{Pattern: antenna.Dipole{}, Polarisation: antenna.Horizontal}
	c := antenna.Mounted{Pattern: antenna.Dipole{}, Polarisation: antenna.Circular}
	unstated := antenna.Mounted{Pattern: antenna.Dipole{}}

	if got := antenna.MismatchLossDB(v, v); got != 0 {
		t.Errorf("two verticals cost %v dB", got)
	}
	if got := antenna.MismatchLossDB(v, h); got != 20 {
		t.Errorf("vertical against horizontal cost %v dB, wanted 20", got)
	}
	if got := antenna.MismatchLossDB(v, c); got != 3 {
		t.Errorf("circular against linear cost %v dB, wanted the classic 3", got)
	}
	// The one that matters for an older scenario: a blank field is a network
	// nobody described, not a network mounted sideways.
	if got := antenna.MismatchLossDB(v, unstated); got != 0 {
		t.Errorf("an unstated polarisation cost %v dB; every link in a scenario "+
			"saved before anybody chose would go off the air", got)
	}
}
