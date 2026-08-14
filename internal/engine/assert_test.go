package engine_test

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/engine"
)

// An assertion that fails must say why. A verdict with no evidence sends
// somebody back to the log the assertion exists to replace.
func TestFailingAssertionsExplainThemselves(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10, FreqMHz: 869.525,
		BandwidthHz: 250e3, SF: 10, CodingRate: 1, NoiseFigDB: 6})
	e.Add(node("a", 56.70, -3.90, 22), nil)
	e.Add(node("b", 56.70, -3.89, 22), nil)

	results := e.Check([]engine.Assertion{
		{Kind: engine.AssertReceives, Node: "b", WithinMs: 5000},
		{Kind: engine.AssertDutyBelow, MaxPct: 1},
	})
	if len(results) != 2 {
		t.Fatalf("got %d results for 2 assertions", len(results))
	}
	if results[0].Passed {
		t.Error("b received nothing, but the assertion passed")
	}
	if results[0].Detail == "" {
		t.Error("a failure with no explanation is a failure nobody can act on")
	}
	// A quiet network is compliant, and saying so is not the same as saying
	// nothing happened.
	if !results[1].Passed {
		t.Errorf("a silent network exceeded a duty limit: %s", results[1].Detail)
	}
}

// The first divergence is the number that matters in an A/B: two totals say
// something changed, only the first divergence says where to look.
func TestDivergeFindsTheFirstRealDifference(t *testing.T) {
	a := []engine.Event{
		{AtMs: 100, Kind: "tx", From: "n1", Detail: "20 bytes, 60 ms on air"},
		{AtMs: 160, Kind: "rx", From: "n1", To: "n2", Detail: "first time this node heard the message"},
		{AtMs: 300, Kind: "tx", From: "n2", Detail: "20 bytes, 60 ms on air"},
	}
	b := []engine.Event{
		{AtMs: 100, Kind: "tx", From: "n1", Detail: "20 bytes, 61 ms on air"},
		{AtMs: 161, Kind: "rx", From: "n1", To: "n2", Detail: "first time this node heard the message"},
		{AtMs: 305, Kind: "miss", From: "n1", To: "n2", Detail: "its own transmitter was keyed; LoRa is half duplex"},
	}
	d := engine.Diverge(a, b)
	if !d.Found {
		t.Fatal("two runs that behaved differently were reported identical")
	}
	if d.AtMs != 300 {
		// The first two events differ only in numbers that legitimately vary;
		// flagging those would cry wolf on every comparison.
		t.Errorf("diverged at %d ms; the first different decision is at 300", d.AtMs)
	}
}
