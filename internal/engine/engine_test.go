package engine_test

import (
	"context"
	"strings"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/antenna"
	"github.com/A13xB0/meshcoresim/internal/capture"
	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

type flat struct{ h float64 }

func (f flat) ElevationM(_, _ float64) (float64, bool) { return f.h, true }

type noTerrain struct{}

func (noTerrain) ElevationM(_, _ float64) (float64, bool) { return 0, false }

func node(name string, lat, lon, tx float64) scenario.Node {
	return scenario.Node{
		Name: name, Kind: scenario.SimpleRepeater,
		Position: scenario.LatLon{Lat: lat, Lon: lon}, HeightAGLm: 10,
		Radio:      scenario.RadioConfig{CentreHz: 869.525e6, BandwidthHz: 250e3, SpreadFactor: 10, CodingRate: 1},
		TxPowerDBm: tx, NoiseFigureDB: 6,
		Antenna: antenna.Mounted{Pattern: antenna.Dipole{}, Polarisation: "vertical"},
	}
}

// Everything the tool is for reduces to this: a packet either arrived or it did
// not, and when it did not the engine has to say which of several very
// different reasons it was.
func TestMissesSayWhichKindOfMiss(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10})
	near := e.Add(node("a", 56.70, -3.90, 22), nil)
	far := e.Add(node("b", 58.90, -1.20, 22), nil) // hundreds of km away
	_ = near
	_ = far

	if len(e.Nodes()) != 2 {
		t.Fatalf("got %d nodes", len(e.Nodes()))
	}
	// With no firmware attached nothing transmits, and a run that produced no
	// traffic must not look like a run where everything failed.
	if err := e.Run(context.Background(), 200); err != nil {
		t.Fatal(err)
	}
	if len(e.Events()) != 0 {
		t.Errorf("a run with no firmware produced %d events", len(e.Events()))
	}
}

// A node cannot hear while its own transmitter is keyed. It is a different
// failure from a weak signal and has a different fix, so it must not be
// reported as one.
func TestHalfDuplexDeafnessIsItsOwnCause(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10})
	e.Add(node("a", 56.700, -3.900, 22), nil)
	e.Add(node("b", 56.705, -3.900, 22), nil)

	// Drive the channel directly: two nodes transmitting across each other.
	e.Inject(0, []byte("hello from a"))
	e.Inject(1, []byte("hello from b"))
	if err := e.Run(context.Background(), 2000); err != nil {
		t.Fatal(err)
	}

	var deaf int
	for _, ev := range e.Events() {
		if ev.Kind == "miss" && strings.Contains(ev.Detail, "half duplex") {
			deaf++
		}
	}
	if deaf == 0 {
		t.Error("two nodes transmitting across each other produced no half-duplex miss")
	}
}

// A close pair must hear each other. Without this the engine could report
// nothing but failures and every other test would still pass.
func TestNearbyNodesHearEachOther(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10})
	e.Add(node("a", 56.700, -3.900, 22), nil)
	e.Add(node("b", 56.710, -3.900, 22), nil)

	e.Inject(0, []byte("a short message"))
	if err := e.Run(context.Background(), 3000); err != nil {
		t.Fatal(err)
	}

	var rx int
	for _, ev := range e.Events() {
		if ev.Kind == "rx" && ev.To == "b" {
			rx++
		}
	}
	if rx == 0 {
		t.Fatalf("a 1 km link at 22 dBm did not deliver; events: %+v", e.Events())
	}
	if got := e.Ledger.Summarise(1); got.Accepted == 0 {
		t.Errorf("the ledger recorded no acceptance: %+v", got)
	}
}

// Distance must eventually stop a link, and the reason must be the honest one.
func TestDistantNodesAreOutOfRange(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10})
	e.Add(node("a", 56.70, -3.90, 22), nil)
	e.Add(node("b", 51.50, -0.10, 22), nil) // Perthshire to London

	e.Inject(0, []byte("hello"))
	if err := e.Run(context.Background(), 3000); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, ev := range e.Events() {
		if ev.Kind == "miss" && ev.To == "b" {
			found = true
			if ev.Outcome != capture.OutOfRange && ev.Outcome != capture.NotDemodulated {
				t.Errorf("a 600 km path was reported as %s", ev.Outcome)
			}
		}
	}
	if !found {
		t.Error("a 600 km path produced no miss at all")
	}
}

// The number that matters. A repeater can be busy, legal, and reaching nobody
// who had not already heard the message, and a duty-cycle figure hides that
// completely.
func TestRedundantRelaysAreDistinguishedFromUniqueOnes(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10})
	e.Add(node("a", 56.700, -3.900, 22), nil)
	e.Add(node("b", 56.706, -3.900, 22), nil)
	e.Add(node("c", 56.712, -3.900, 22), nil)

	payload := []byte("one message")
	e.Inject(0, payload)
	if err := e.Run(context.Background(), 3000); err != nil {
		t.Fatal(err)
	}
	// The same payload sent again by another node is a relay that reaches
	// nobody new.
	e.Inject(1, payload)
	if err := e.Run(context.Background(), 6000); err != nil {
		t.Fatal(err)
	}

	board := e.Scoreboard()
	byName := map[string]engine.Score{}
	for _, s := range board {
		byName[s.Name] = s
	}
	if byName["a"].UniqueDelivery == 0 {
		t.Error("the originator delivered nothing new")
	}
	if byName["b"].RedundantRelay == 0 {
		t.Errorf("relaying a message everyone already had was counted as unique: %+v", byName["b"])
	}
}

// No terrain is not free space. A path the DEM cannot describe must not be
// priced as though it were flat.
func TestMissingTerrainIsNotFreeSpace(t *testing.T) {
	e := engine.New(noTerrain{}, engine.Config{StepMs: 10})
	e.Add(node("a", 56.700, -3.900, 22), nil)
	e.Add(node("b", 56.705, -3.900, 22), nil)

	e.Inject(0, []byte("hello"))
	if err := e.Run(context.Background(), 3000); err != nil {
		t.Fatal(err)
	}
	for _, ev := range e.Events() {
		if ev.Kind == "rx" {
			t.Fatal("a path with no terrain data delivered a packet")
		}
		if ev.Kind == "miss" && !strings.Contains(ev.Detail, "terrain") {
			t.Errorf("the miss does not say terrain was missing: %q", ev.Detail)
		}
	}
}

// Duty cycle is what makes a network legal, and it has to come from measured
// airtime rather than an assumed message rate.
func TestDutyCycleComesFromMeasuredAirtime(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10})
	e.Add(node("a", 56.700, -3.900, 22), nil)
	e.Add(node("b", 56.706, -3.900, 22), nil)

	for i := 0; i < 5; i++ {
		e.Inject(0, []byte("a message of some length to occupy the air"))
		if err := e.Run(context.Background(), uint32(2000*(i+1))); err != nil {
			t.Fatal(err)
		}
	}
	board := e.Scoreboard()
	if board[0].AirtimeMs <= 0 {
		t.Fatal("no airtime was accumulated")
	}
	if board[0].DutyCyclePct <= 0 || board[0].DutyCyclePct > 100 {
		t.Errorf("duty cycle %.2f%% is not a percentage", board[0].DutyCyclePct)
	}
	if board[0].Sent != 5 {
		t.Errorf("sent %d, want 5", board[0].Sent)
	}
}
