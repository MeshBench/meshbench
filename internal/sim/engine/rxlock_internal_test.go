package engine

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// lockBench is three nodes on flat ground: a receiver and two transmitters at
// different ranges, so one is decisively stronger at the receiver than the
// other. Positions are close enough that both are heard easily.
func lockBench(t *testing.T) (*Engine, phy) {
	t.Helper()
	e := New(flatEarth{}, Config{StepMs: 10, Seed: 4417})
	mk := func(name string, lonOffset float64) scenario.Node {
		n := scenario.Node{Name: name, Kind: scenario.SimpleRepeater}
		n.Position.Lat, n.Position.Lon = 56.2, -3.16+lonOffset
		n.TxPowerDBm = 22
		n.HeightAGLm = 8
		n.Radio.SpreadFactor = 8
		n.Radio.BandwidthHz = 62.5e3
		n.Radio.CodingRate = 4
		return n
	}
	e.Add(mk("listener", 0), nil)
	e.Add(mk("near", 0.001), nil)  // ~60 m: the dominant signal
	e.Add(mk("far", 0.050), nil)   // ~3 km: heard, but well down on near
	e.Add(mk("other", 0.020), nil) // ~1.2 km: a third voice
	return e, e.phyOf(e.nodes[0].Spec)
}

// A transmission that lost the demodulator holds nothing afterwards.
//
// The first lock model asked one pairwise question - "did anything detectable
// start before this and overlap it" - which let a packet that had itself lost
// the contest go on blocking every later packet it happened to overlap. In a
// mesh's continuous traffic those phantoms chained, and a fixture that had
// been delivering thousands of receptions delivered fifty-three: the
// companion could not hear its own message come back.
//
// Here `far` is beaten by `near` at the start and its preamble is long gone
// by the time the demodulator frees, so it never holds the lock at all - and
// must not stand in the way of `other`, which arrives to an empty receiver.
func TestALostContenderHoldsNothing(t *testing.T) {
	e, p := lockBench(t)
	nodes := e.nodes

	// near dominates the contest at t=0 and holds until 500.
	near := transmission{from: 1, packetID: 1, startMs: 0, endMs: 500}
	// far starts inside near's hold and ends before it frees: lost outright.
	far := transmission{from: 2, packetID: 2, startMs: 200, endMs: 1000}
	// other arrives after near has finished, while far is nominally still on
	// the air - and with too little preamble left to have waited far out. If
	// far still held the demodulator it would block this; far does not hold
	// it, so the air is empty and this is heard.
	other := transmission{from: 3, packetID: 3, startMs: 850, endMs: 1350}
	concurrent := []transmission{near, far, other}

	if held := e.demodulatorHeldBy(0, other, concurrent, nodes, p); held != "" {
		t.Fatalf("a packet arriving to a free receiver was blocked by %q; "+
			"the holder had itself lost the demodulator", held)
	}
	// And the winner of the original contest is still the winner.
	if held := e.demodulatorHeldBy(0, far, concurrent, nodes, p); held != "near" {
		t.Fatalf("far was blocked by %q, want near - the packet that actually "+
			"took the lock", held)
	}
	if held := e.demodulatorHeldBy(0, near, concurrent, nodes, p); held != "" {
		t.Fatalf("the dominant first arrival was itself blocked, by %q", held)
	}
}

// Back-to-back traffic is heard back to back. Each packet ends before the
// next begins, so every one of them finds a free demodulator - the case a
// phantom hold breaks first and most visibly, because it is most of what a
// quiet mesh does.
func TestSequentialTrafficIsAllHeard(t *testing.T) {
	e, p := lockBench(t)
	nodes := e.nodes

	var concurrent []transmission
	for i := 0; i < 6; i++ {
		concurrent = append(concurrent, transmission{
			from: 1 + i%3, packetID: uint64(i + 1),
			startMs: uint32(i * 600), endMs: uint32(i*600 + 500),
		})
	}
	for _, tr := range concurrent {
		if held := e.demodulatorHeldBy(0, tr, concurrent, nodes, p); held != "" {
			t.Fatalf("packet %d, alone on the air from %d to %d ms, was blocked by %q",
				tr.packetID, tr.startMs, tr.endMs, held)
		}
	}
}
