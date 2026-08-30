package engine_test

import (
	"context"
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/rf/geo"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A beam is a beam. Turned to face the far end it buys the link its headline
// gain; turned away it costs the front-to-back ratio, and on a link with
// little to spare that is the difference between a packet and no packet.
// Peak gain charged regardless of where the antenna points cannot express
// either, which is what the engine used to do.
func TestATurnedYagiChangesTheOutcome(t *testing.T) {
	run := func(boresightDeg float64) string {
		e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 31})
		// The thin-margin geometry the realism tests use: clean over bare
		// earth, with nothing to spare.
		a := wfNode("a", 0, 22)
		a.Antenna = antenna.Mounted{
			Pattern:      antenna.Yagi{GainDBiPeak: 10, BeamwidthDeg: 50, FrontToBackDB: 20},
			BearingDeg:   boresightDeg,
			Polarisation: antenna.Vertical,
		}
		e.Add(a, nil)
		e.Add(wfNode("b", 0.80, 22), nil)
		_ = e.Run(context.Background(), 10)
		e.InjectFrame(0, []byte("which way is the beam pointing"))
		_ = e.Run(context.Background(), 8000)
		for _, ev := range e.Events() {
			if ev.From == "a" && ev.To == "b" {
				return ev.Kind
			}
		}
		return "nothing"
	}
	// b is due east of a.
	if got := run(90); got != "rx" {
		t.Fatalf("the beam pointed at the far end did not decode: %s", got)
	}
	if got := run(270); got == "rx" {
		t.Fatal("the beam turned 180 degrees away cost the link nothing")
	}
}

// A collinear buys its gain at the horizon by squashing the pattern
// vertically, so a repeater looking steeply down at a node below it is not
// working at its headline figure. That loss is the ordinary case for this
// tool - hills - and it was not being charged at all.
func TestElevationCostsWhatThePatternSays(t *testing.T) {
	const peak = 10.0
	mast := func(name string, lonOffset, heightM float64) scenario.Node {
		n := wfNode(name, lonOffset, 22)
		n.HeightAGLm = heightM
		n.Antenna = antenna.Mounted{
			Pattern: antenna.Collinear{GainDBiPeak: peak}, Polarisation: antenna.Vertical,
		}
		return n
	}
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 32})
	e.Add(mast("hill", 0, 500), nil)   // on a mast 500 m up
	e.Add(mast("glen", 0.08, 10), nil) // ~5 km east, at 10 m

	up, down := e.LinkGainsDBiForTest(0, 1)
	distKm := geo.DistanceKm(56.700, -3.900, 56.700, -3.900+0.08)
	elev := geo.ElevationDeg(100+500, 100+10, distKm)
	want := antenna.Collinear{GainDBiPeak: peak}.GainDBi(0, elev)

	if math.Abs(up-want) > 0.01 {
		t.Errorf("looking %.2f degrees down: gain %.2f dBi, the pattern says %.2f",
			elev, up, want)
	}
	if peak-up < 5 {
		t.Errorf("a %.0f degree look angle cost only %.2f dB of a %.0f dBi collinear; "+
			"the elevation is not reaching the pattern", elev, peak-up, peak)
	}
	// Asymmetric by construction: the glen looks up at the hill by as much as
	// the hill looks down, so both ends pay - and each pays its own way.
	if math.Abs(down-antenna.Collinear{GainDBiPeak: peak}.GainDBi(0, -elev)) > 0.01 {
		t.Errorf("the far end's gain %.2f dBi is not its own look angle", down)
	}
}
