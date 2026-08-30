package engine

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Two transmitters that the link budget calls undetectable, keyed together.
//
// demodulatorHeldBy admits the transmission under test on different terms from
// every other candidate: t needs only a receive power, while every other has
// additionally to clear detectableAt. So when both are below that threshold,
// each is the only candidate in its own contest, each wins, and both are told
// the demodulator was free. The receiver then decodes both halves of a
// collision - which is what tools/soak caught once in 30,000 events (#125).
// Skipped, deliberately. It reproduces #125 rather than guarding a fix, and
// what the fix should be is a question about the model rather than a typo: the
// blocking gate has to admit everything the waveform chain can decode - which
// is up to 1.6 dB below the link-budget floor, per docs/shortcomings.md 2.1 -
// without letting a carrier far under the noise floor hold a receiver.
// Unskipping this is the acceptance criterion for whatever that gate becomes.
func TestSubThresholdArrivalsBothWin(t *testing.T) {
	t.Skip("reproduces #125: the lock admits t on different terms from every other candidate")
	e := New(flatEarth{}, Config{StepMs: 10, Seed: 4417})
	mk := func(name string, kind scenario.Kind, lat, lon, agl, tx float64) scenario.Node {
		n := scenario.Node{Name: name, Kind: kind}
		n.Position.Lat, n.Position.Lon = lat, lon
		n.HeightAGLm, n.TxPowerDBm, n.NoiseFigureDB = agl, tx, 6
		n.Radio.SpreadFactor, n.Radio.BandwidthHz, n.Radio.CodingRate = 8, 62.5e3, 4
		n.Radio.CentreHz = 869618000
		return n
	}
	// Far enough, and quiet enough, that neither clears the detection floor.
	e.Add(mk("listener", scenario.SDRObserver, 56.0, -3.0, 2, 0), nil)
	e.Add(mk("far-a", scenario.SimpleRepeater, 57.4, -3.0, 2, -20), nil)
	e.Add(mk("far-b", scenario.SimpleRepeater, 56.0, -1.0, 2, -20), nil)
	nodes := e.nodes
	p := e.phyOf(nodes[1].Spec())

	a := transmission{from: 1, packetID: 1, startMs: 1000, endMs: 1541}
	b := transmission{from: 2, packetID: 2, startMs: 1000, endMs: 1541}
	concurrent := []transmission{a, b}

	_, aDetectable := e.detectableAt(0, a, nodes, p)
	_, bDetectable := e.detectableAt(0, b, nodes, p)
	t.Logf("detectable at the listener: a=%v b=%v", aDetectable, bDetectable)

	heldA := e.demodulatorHeldBy(0, a, concurrent, nodes, p)
	heldB := e.demodulatorHeldBy(0, b, concurrent, nodes, p)
	t.Logf("heldBy: a=%q b=%q", heldA, heldB)

	if heldA == "" && heldB == "" {
		t.Fatalf("both transmissions were told the demodulator was free; "+
			"one receiver cannot decode two fully-overlapping same-channel "+
			"frames (detectable: a=%v b=%v)", aDetectable, bDetectable)
	}
}
