package engine_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/MeshBench/meshbench/internal/dsp"
	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// wfNode is a close-range node for waveform tests: near enough that free
// space leaves a strong signal, far enough apart to be distinct.
func wfNode(name string, lonOffset, tx float64) scenario.Node {
	n := node(name, 56.700, -3.900+lonOffset, tx)
	n.Radio.SpreadFactor = 8
	n.Radio.BandwidthHz = 125e3
	return n
}

// runCollision plays one wanted transmission from a, with an equal-power
// interferer from c starting at interfererStartMs, and reports b's outcome
// for a's packet.
func runCollision(t *testing.T, mode engine.RFMode, interfererStartMs uint32) string {
	t.Helper()
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417, RFMode: mode})
	e.Add(wfNode("a", 0, 22), nil)
	e.Add(wfNode("b", 0.010, 22), nil)
	e.Add(wfNode("c", 0.020, 22), nil)

	frame := make([]byte, 40)
	for i := range frame {
		frame[i] = byte(37 + i*11)
	}
	// The wanted packet goes on the air at t=10ms (first step).
	if err := e.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	e.InjectFrame(0, frame)
	if err := e.Run(context.Background(), interfererStartMs); err != nil {
		t.Fatal(err)
	}
	e.InjectFrame(2, frame[:20])
	if err := e.Run(context.Background(), 5000); err != nil {
		t.Fatal(err)
	}

	for _, ev := range e.Events() {
		if ev.From == "a" && ev.To == "b" {
			return ev.Kind
		}
	}
	return "nothing"
}

// The MS1 gate: an equal-power collision that dBm arithmetic waves through
// identically in both alignments, and only the demodulator tells apart.
//
// The interferer either overlaps the wanted packet or has already finished.
// The calculated model treats any overlap as full-strength interference and
// no overlap as none - and at these powers its verdict is the same either
// way. The waveform verdict differs, because alignment is real there.
func TestWaveformInterferenceAlignmentDecides(t *testing.T) {
	const overlapping, clear = 20, 4000 // interferer start, ms

	calcSame := runCollision(t, engine.RFCalculated, overlapping)
	calcClear := runCollision(t, engine.RFCalculated, clear)
	if calcSame != calcClear {
		t.Fatalf("calculated mode distinguished the alignments (%q vs %q); "+
			"the test needs a collision it waves through", calcSame, calcClear)
	}

	wfSame := runCollision(t, engine.RFWaveform, overlapping)
	wfClear := runCollision(t, engine.RFWaveform, clear)
	if wfSame == wfClear {
		t.Fatalf("waveform mode did not distinguish overlap (%q) from clear air (%q)",
			wfSame, wfClear)
	}
	if wfClear != "rx" {
		t.Fatalf("clear air should decode, got %q", wfClear)
	}
	if wfSame != "miss" {
		t.Fatalf("an equal-power full collision should not decode, got %q", wfSame)
	}
}

// Changing the demodulator-floor table must change nothing in waveform mode:
// if an SNR threshold changes the answer, the old model is still in charge.
func TestRequiredSNRIsIrrelevantInWaveformMode(t *testing.T) {
	before := runCollision(t, engine.RFWaveform, 20)

	saved := map[int]float64{}
	for sf, v := range dsp.RequiredSNRdB {
		saved[sf] = v
		dsp.RequiredSNRdB[sf] = v + 40 // absurd: nothing would ever decode
	}
	defer func() {
		for sf, v := range saved {
			dsp.RequiredSNRdB[sf] = v
		}
	}()

	after := runCollision(t, engine.RFWaveform, 20)
	if before != after {
		t.Fatalf("shifting the SNR floor changed a waveform verdict (%q -> %q): "+
			"the threshold is still in charge", before, after)
	}
}

// Capture must emerge from the FFT, not from a constant: a wanted signal
// well above an interferer decodes through the collision.
func TestWaveformCaptureEmerges(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417, RFMode: engine.RFWaveform})
	e.Add(wfNode("a", 0.001, 22), nil) // ~70 m from b: strong
	e.Add(wfNode("b", 0, 22), nil)
	e.Add(wfNode("c", 0.060, 22), nil) // ~4 km from b: much weaker at b

	frame := make([]byte, 40)
	for i := range frame {
		frame[i] = byte(200 - i)
	}
	if err := e.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	e.InjectFrame(0, frame)
	e.InjectFrame(2, frame[:30]) // fully inside a's airtime
	if err := e.Run(context.Background(), 5000); err != nil {
		t.Fatal(err)
	}
	for _, ev := range e.Events() {
		if ev.From == "a" && ev.To == "b" {
			if ev.Kind != "rx" {
				t.Fatalf("the much stronger signal did not capture: %s (%s)", ev.Kind, ev.Detail)
			}
			return
		}
	}
	t.Fatal("no event from a to b at all")
}

// Same seed, same scenario, same events - waveform mode keeps the project's
// determinism rule, noise and all.
func TestWaveformModeIsDeterministic(t *testing.T) {
	run := func() string {
		e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 99, RFMode: engine.RFWaveform})
		e.Add(wfNode("a", 0, 22), nil)
		e.Add(wfNode("b", 0.010, 22), nil)
		e.Add(wfNode("c", 0.020, 22), nil)
		if err := e.Run(context.Background(), 10); err != nil {
			t.Fatal(err)
		}
		e.InjectFrame(0, []byte("determinism is a feature"))
		e.InjectFrame(2, []byte("and this is the interferer"))
		if err := e.Run(context.Background(), 5000); err != nil {
			t.Fatal(err)
		}
		out := ""
		for _, ev := range e.Events() {
			out += fmt.Sprintf("%d %s %s>%s %s %.4f\n",
				ev.AtMs, ev.Kind, ev.From, ev.To, ev.Detail, ev.SNRdB)
		}
		return out
	}
	a, b := run(), run()
	if a != b {
		t.Fatalf("two identical runs disagreed:\n--- first\n%s--- second\n%s", a, b)
	}
	if a == "" {
		t.Fatal("the runs produced no events at all")
	}
}

// A clean, strong, uncontested packet must decode in waveform mode - the
// baseline sanity every other test leans on.
func TestWaveformCleanPacketDecodes(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 1, RFMode: engine.RFWaveform})
	e.Add(wfNode("a", 0, 22), nil)
	e.Add(wfNode("b", 0.010, 22), nil)
	if err := e.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	e.InjectFrame(0, []byte("hello waveform world"))
	if err := e.Run(context.Background(), 5000); err != nil {
		t.Fatal(err)
	}
	for _, ev := range e.Events() {
		if ev.From == "a" && ev.To == "b" && ev.Kind == "rx" {
			return
		}
	}
	t.Fatalf("clean packet not received; events: %+v", e.Events())
}
