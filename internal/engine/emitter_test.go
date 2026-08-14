package engine_test

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/antenna"
	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/scenario"
)

func emitter(name string, lat, lon, erpDBm, centreHz, bwHz float64) scenario.Node {
	return scenario.Node{
		Name: name, Kind: scenario.Emitter,
		Position: scenario.LatLon{Lat: lat, Lon: lon}, HeightAGLm: 20,
		Radio:          scenario.RadioConfig{CentreHz: centreHz, BandwidthHz: bwHz},
		TxPowerDBm:     erpDBm,
		EmitterDutyPct: 100,
		Antenna:        antenna.Mounted{Pattern: antenna.Dipole{}, Polarisation: "vertical"},
	}
}

func deliveredCount(t *testing.T, extra ...scenario.Node) int {
	t.Helper()
	e := engine.New(flat{100}, engine.Config{StepMs: 10})
	defer func() { _ = e.Close() }()
	e.Add(node("a", 56.700, -3.900, 22), nil)
	e.Add(node("b", 56.705, -3.905, 22), nil)
	for _, n := range extra {
		e.Add(n, nil)
	}
	e.Inject(0, []byte("hello"))
	if err := e.Run(context.Background(), 400); err != nil {
		t.Fatal(err)
	}
	rx := 0
	for _, ev := range e.Events() {
		if ev.Kind == "rx" && ev.To == "b" {
			rx++
		}
	}
	return rx
}

// ADR-0012's core claim: an in-band emitter beside the receiver raises its
// floor and kills a link that otherwise works, and an out-of-band one does
// not — because only power inside the passband counts here.
func TestEmitterRaisesTheFloorInBandOnly(t *testing.T) {
	if deliveredCount(t) == 0 {
		t.Fatal("the link does not work even without an emitter; the test measures nothing")
	}
	// A strong wideband emitter on-channel, a few hundred metres from b.
	inBand := emitter("mast", 56.706, -3.906, 50, 869.525e6, 250e3)
	if got := deliveredCount(t, inBand); got != 0 {
		t.Fatalf("a 50 dBm on-channel emitter next to the receiver left %d receptions", got)
	}
	// The same mast on a different band: no overlap, no contribution.
	outOfBand := emitter("mast", 56.706, -3.906, 50, 446.0e6, 250e3)
	if got := deliveredCount(t, outOfBand); got == 0 {
		t.Fatal("an out-of-band emitter silenced the link; overlap is being ignored")
	}
}

// The per-receiver floor is queryable, and a node near the mast reports a
// higher floor than one far from it.
func TestFloorAtIsPerReceiver(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10})
	defer func() { _ = e.Close() }()
	e.Add(node("near", 56.700, -3.900, 22), nil)
	e.Add(node("far", 57.400, -2.900, 22), nil)
	e.Add(emitter("mast", 56.701, -3.901, 44, 869.525e6, 250e3), nil)

	thermalN, withN, ok := e.FloorAt("near")
	if !ok {
		t.Fatal("no floor for near")
	}
	_, withF, ok := e.FloorAt("far")
	if !ok {
		t.Fatal("no floor for far")
	}
	if withN <= thermalN {
		t.Fatalf("near the mast the floor did not rise: thermal %.1f, with %.1f", thermalN, withN)
	}
	if withN <= withF {
		t.Fatalf("the floor is not per-receiver: near %.1f, far %.1f", withN, withF)
	}
}
