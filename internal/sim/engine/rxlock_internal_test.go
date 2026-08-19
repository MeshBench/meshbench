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

// Two transmissions that start together and end together: exactly one of them
// gets the demodulator.
//
// The soak found a receiver decoding both halves of precisely this - two
// relays keyed on the same millisecond, both 541 ms long, both landing at
// within two thousandths of a decibel of each other. Whichever is stronger
// wins; the other must be told who took it. Symmetry is not an excuse for
// two answers.
func TestSimultaneousArrivalsLeaveOneWinner(t *testing.T) {
	e, p := lockBench(t)
	nodes := e.nodes

	a := transmission{from: 1, packetID: 10, startMs: 51720, endMs: 52261}
	b := transmission{from: 2, packetID: 11, startMs: 51720, endMs: 52261}
	concurrent := []transmission{a, b}

	heldA := e.demodulatorHeldBy(0, a, concurrent, nodes, p)
	heldB := e.demodulatorHeldBy(0, b, concurrent, nodes, p)
	if (heldA == "") == (heldB == "") {
		t.Fatalf("both packets got the same answer (a:%q b:%q); one demodulator "+
			"means exactly one winner", heldA, heldB)
	}
	// And the winner is the stronger of the two: `near` beats `far`.
	if heldA != "" {
		t.Fatalf("the stronger arrival was blocked by %q", heldA)
	}
	if heldB != "near" {
		t.Fatalf("the weaker arrival was blocked by %q, want near", heldB)
	}
}

// An SDR observer is wideband, and wideband is not the same as unlimited.
//
// The soak caught one decoding two fully-overlapping same-channel relays -
// 541 ms each, keyed on the same millisecond. Whatever exemptions an
// observer earns for watching channels its own mesh is not on, one receive
// chain still demodulates one packet at a time on one channel.
func TestAnSDRObserverStillHasOneDemodulator(t *testing.T) {
	e, p := lockBench(t)
	obs := scenario.Node{Name: "watcher", Kind: scenario.SDRObserver}
	obs.Position.Lat, obs.Position.Lon = 56.2, -3.16
	obs.TxPowerDBm, obs.HeightAGLm = 22, 8
	obs.Radio.SpreadFactor, obs.Radio.BandwidthHz, obs.Radio.CodingRate = 8, 62.5e3, 4
	e.Add(obs, nil)
	nodes := e.nodes
	rx := len(nodes) - 1

	a := transmission{from: 1, packetID: 10, startMs: 51720, endMs: 52261}
	b := transmission{from: 2, packetID: 11, startMs: 51720, endMs: 52261}
	concurrent := []transmission{a, b}

	heldA := e.demodulatorHeldBy(rx, a, concurrent, nodes, p)
	heldB := e.demodulatorHeldBy(rx, b, concurrent, nodes, p)
	if (heldA == "") == (heldB == "") {
		t.Fatalf("the observer gave both packets the same answer (a:%q b:%q)",
			heldA, heldB)
	}
}

// The soak's own case, at its own coordinates.
//
// Two Fife repeaters relayed on the same millisecond, 541 ms each, and the
// Kirkcaldy observer twenty-odd kilometres from both decoded them both -
// their received powers differing by under two thousandths of a decibel.
// Near-equal is where a contest resolved by comparison is most fragile, so
// this pins the real geometry rather than a convenient one.
func TestTheSoaksOwnDoubleDecode(t *testing.T) {
	e := New(flatEarth{}, Config{StepMs: 10, Seed: 4417})
	mk := func(name string, kind scenario.Kind, lat, lon, agl, tx float64) scenario.Node {
		n := scenario.Node{Name: name, Kind: kind}
		n.Position.Lat, n.Position.Lon = lat, lon
		n.HeightAGLm, n.TxPowerDBm, n.NoiseFigureDB = agl, tx, 6
		n.Radio.SpreadFactor, n.Radio.BandwidthHz, n.Radio.CodingRate = 8, 62.5e3, 4
		n.Radio.CentreHz = 869618000
		return n
	}
	e.Add(mk("Kirkcaldy SDR Observer", scenario.SDRObserver, 56.1128, -3.158, 12, 0), nil)
	e.Add(mk("Drumcarrow Craig NWR", scenario.SimpleRepeater, 56.308243, -2.874867, 10, 22), nil)
	e.Add(mk("sco-fif-montrave", scenario.SimpleRepeater, 56.246614, -3.03611, 10, 22), nil)
	nodes := e.nodes
	p := e.phyOf(nodes[1].Spec)

	a := transmission{from: 1, packetID: 1253, startMs: 51720, endMs: 52261}
	b := transmission{from: 2, packetID: 1256, startMs: 51720, endMs: 52261}
	concurrent := []transmission{a, b}

	heldA := e.demodulatorHeldBy(0, a, concurrent, nodes, p)
	heldB := e.demodulatorHeldBy(0, b, concurrent, nodes, p)
	if (heldA == "") == (heldB == "") {
		t.Fatalf("the observer answered both the same (a:%q b:%q); two "+
			"simultaneous relays cannot both have the demodulator", heldA, heldB)
	}
}
