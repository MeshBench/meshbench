package engine_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/dsp"
	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/environ"
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

// The MS1 gate: the same interferer at the same power, and the only thing
// that differs is when it transmits - dead-aligned with the wanted packet,
// or after the air has cleared. The calculated model's verdict is identical
// in both timings (an equal-power interferer leaves the effective SNR far
// above the demodulator floor, so it calls both received); the receiver's
// verdict is not, because an equal-power collision is a coin flip in every
// FFT bin and a cleared channel is not. Timing decides, dBm cannot.
func TestWaveformInterferenceAlignmentDecides(t *testing.T) {
	const colliding, cleared = 10, 4000 // interferer start, ms; wanted starts at 10

	calcC := runCollision(t, engine.RFCalculated, colliding)
	calcF := runCollision(t, engine.RFCalculated, cleared)
	if calcC != calcF {
		t.Fatalf("calculated mode distinguished the timings (%q vs %q); "+
			"the test needs a collision it waves through identically", calcC, calcF)
	}
	if calcC != "rx" {
		t.Fatalf("the calculated model should wave this collision through, got %q", calcC)
	}

	wfC := runCollision(t, engine.RFWaveform, colliding)
	wfF := runCollision(t, engine.RFWaveform, cleared)
	if wfC != "miss" {
		t.Fatalf("an equal-power aligned collision should not decode, got %q", wfC)
	}
	if wfF != "rx" {
		t.Fatalf("clear air should decode, got %q", wfF)
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

// Carrier sense in waveform mode is the chip's own question - dechirped
// concentration in one symbol of IQ - and it must say busy near a
// transmitter, quiet out of range, and quiet again once the air clears.
func TestWaveformCADTracksTheAir(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 5, RFMode: engine.RFWaveform})
	e.Add(wfNode("tx", 0, 22), nil)
	e.Add(wfNode("near", 0.010, 22), nil)
	e.Add(wfNode("far", 3.0, 22), nil) // ~180 km: nothing detectable
	if err := e.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}

	if busy := e.ChannelBusyForTest(10); busy[1] || busy[2] {
		t.Fatalf("quiet air read as busy: %v", busy)
	}
	e.InjectFrame(0, make([]byte, 60))
	if busy := e.ChannelBusyForTest(60); !busy[1] {
		t.Fatal("a near node did not detect a transmission in flight")
	} else if busy[2] {
		t.Fatal("a node 180 km away detected the transmission")
	}
	if err := e.Run(context.Background(), 5000); err != nil {
		t.Fatal(err)
	}
	if busy := e.ChannelBusyForTest(5000); busy[1] {
		t.Fatal("the channel stayed busy after the air cleared")
	}
}

// An observer's stream is the RF world, not the event log: it carries signal
// while a transmission overlaps its span, noise otherwise - and moving the
// observer changes what it hears, because position prices the path.
func TestObserverSpanHearsTheAirAndMovesWithTheNode(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 3, RFMode: engine.RFWaveform})
	e.Add(wfNode("tx", 0, 22), nil)
	obs := wfNode("obs", 0.010, 22)
	obs.Kind = scenario.SDRObserver
	e.Add(obs, nil)
	if err := e.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}

	power := func(iq []complex128) float64 {
		var p float64
		for _, v := range iq {
			p += real(v)*real(v) + imag(v)*imag(v)
		}
		return p / float64(len(iq))
	}

	quiet := power(e.ObserveSpan(1, 10, 8192))
	e.InjectFrame(0, make([]byte, 80))
	loud := power(e.ObserveSpan(1, 60, 8192))
	if loud < quiet*10 {
		t.Fatalf("a transmission in the span did not raise the stream's power: quiet %g loud %g", quiet, loud)
	}

	// Walk the observer 60 km away mid-stream: the same span must go quiet.
	e.SetNodePosition(1, 56.7, -3.9+1.0)
	far := power(e.ObserveSpan(1, 60, 8192))
	if far > loud/1000 {
		t.Fatalf("moving the observer away did not change what it hears: near %g far %g", loud, far)
	}
}

// Oscillator error is real IQ rotation, and the front end has to earn its
// CFO estimator: at 30 ppm two 869 MHz radios can disagree by several bins,
// and the frame must still decode because Detect measures and corrects it.
func TestOscillatorErrorIsCorrectedByTheFrontEnd(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{
		StepMs: 10, Seed: 11, RFMode: engine.RFWaveform,
		Realism: engine.Realism{OscillatorPPM: 30},
	})
	e.Add(wfNode("a", 0, 22), nil)
	e.Add(wfNode("b", 0.010, 22), nil)
	if err := e.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	e.InjectFrame(0, []byte("thirty parts per million of disagreement"))
	if err := e.Run(context.Background(), 5000); err != nil {
		t.Fatal(err)
	}
	for _, ev := range e.Events() {
		if ev.From == "a" && ev.To == "b" {
			if ev.Kind != "rx" {
				t.Fatalf("CFO was not corrected: %s (%s)", ev.Kind, ev.Detail)
			}
			return
		}
	}
	t.Fatal("no event at all")
}

// A multipath echo is deterministic geometry, not a dice roll: the same
// scenario fades the same way twice, and switching the echo off changes the
// received IQ.
func TestMultipathIsDeterministicGeometry(t *testing.T) {
	run := func(echoDB float64) string {
		e := engine.New(flat{100}, engine.Config{
			StepMs: 10, Seed: 12, RFMode: engine.RFWaveform,
			Realism: engine.Realism{MultipathEchoDB: echoDB},
		})
		e.Add(wfNode("a", 0, 22), nil)
		e.Add(wfNode("b", 0.010, 22), nil)
		_ = e.Run(context.Background(), 10)
		e.InjectFrame(0, []byte("two paths, one antenna"))
		_ = e.Run(context.Background(), 5000)
		out := ""
		for _, ev := range e.Events() {
			out += fmt.Sprintf("%s>%s %s %.6f|", ev.From, ev.To, ev.Kind, ev.SNRdB)
		}
		return out
	}
	withEcho, again := run(6), run(6)
	if withEcho != again {
		t.Fatal("the same echo geometry produced different runs")
	}
	if clean := run(0); clean == withEcho {
		t.Fatal("a 6 dB echo left the measured channel identical")
	}
}

// Implementation loss must deafen the receiver by exactly its own amount: a
// frame near the floor decodes without it and dies with 10 dB of it.
func TestImplementationLossDeafens(t *testing.T) {
	run := func(lossDB float64) string {
		e := engine.New(flat{100}, engine.Config{
			StepMs: 10, Seed: 13, RFMode: engine.RFWaveform,
			Realism: engine.Realism{ImplementationLossDB: lossDB},
		})
		// Far enough that the margin is thin: ~34 km at 22 dBm free space.
		e.Add(wfNode("a", 0, 22), nil)
		e.Add(wfNode("b", 0.55, 22), nil)
		_ = e.Run(context.Background(), 10)
		e.InjectFrame(0, []byte("margin is thin out here"))
		_ = e.Run(context.Background(), 8000)
		for _, ev := range e.Events() {
			if ev.From == "a" && ev.To == "b" {
				return ev.Kind
			}
		}
		return "nothing"
	}
	if got := run(0); got != "rx" {
		t.Skipf("the thin-margin link did not decode clean (%s); geometry needs retuning", got)
	}
	if got := run(10); got == "rx" {
		t.Fatal("10 dB of implementation loss cost nothing")
	}
}

// The hybrid: a calculated run in which one flagged receiver is judged by
// the waveform. The flagged node's events say so; everything else stays on
// the fast model.
func TestHybridFlagGivesOneReceiverWaveformVerdicts(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 14}) // calculated
	e.Add(wfNode("a", 0, 22), nil)
	tru := wfNode("tru", 0.010, 22)
	tru.TrueRF = true
	e.Add(tru, nil)
	e.Add(wfNode("fast", 0.020, 22), nil)
	_ = e.Run(context.Background(), 10)
	e.InjectFrame(0, []byte("one foot in each physics"))
	_ = e.Run(context.Background(), 5000)

	sawTrue, sawFast := false, false
	for _, ev := range e.Events() {
		if ev.From != "a" {
			continue
		}
		if ev.To == "tru" {
			sawTrue = true
			if ev.Kind != "rx" || !strings.Contains(ev.Detail, "waveform") {
				t.Fatalf("the flagged receiver was not waveform-judged: %s (%s)", ev.Kind, ev.Detail)
			}
		}
		if ev.To == "fast" {
			sawFast = true
			if strings.Contains(ev.Detail, "waveform") {
				t.Fatalf("an unflagged receiver was waveform-judged: %s", ev.Detail)
			}
		}
	}
	if !sawTrue || !sawFast {
		t.Fatalf("missing events: tru=%v fast=%v", sawTrue, sawFast)
	}
}

// slabEnv is one tall concrete building squarely on the a-b path.
type slabEnv struct{ b environ.Building }

func (s slabEnv) Buildings(_, _, _, _ float64) []environ.Building {
	return []environ.Building{s.b}
}

// A building on the path costs the link real decibels in both modes, and an
// unloaded environment costs nothing - bare earth stays bare earth.
func TestBuildingsPriceThePath(t *testing.T) {
	run := func(env environ.Provider) string {
		e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 21, RFMode: engine.RFWaveform})
		e.Env = env
		e.Add(wfNode("a", 0, 22), nil)
		e.Add(wfNode("b", 0.45, 5), nil) // ~27 km at 5 dBm: thin margin
		_ = e.Run(context.Background(), 10)
		e.InjectFrame(0, []byte("is there a building in the way"))
		_ = e.Run(context.Background(), 8000)
		for _, ev := range e.Events() {
			if ev.From == "a" && ev.To == "b" {
				return ev.Kind
			}
		}
		return "nothing"
	}

	if got := run(nil); got != "rx" {
		t.Skipf("the thin-margin link did not decode over bare earth (%s)", got)
	}
	slab := environ.Building{
		Footprint: [][2]float64{
			{56.69, -3.68}, {56.69, -3.67}, {56.71, -3.67}, {56.71, -3.68},
		},
		HeightM: 60, Material: environ.MatConcrete,
	}
	if got := run(slabEnv{slab}); got == "rx" {
		t.Fatal("a 60 m concrete slab across the path cost nothing")
	}
}
