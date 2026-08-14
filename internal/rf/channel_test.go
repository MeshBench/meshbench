package rf

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/dsp"
)

const sf = 8

func symbol(v int) []complex128 { return dsp.Modulator{SF: sf}.ModulateSymbol(v) }
func demod(x []complex128) (int, float64) {
	return dsp.Demodulator{SF: sf}.DemodulateSymbol(x)
}

// Capture effect must EMERGE. Nothing in package rf knows what a collision is;
// two overlapping frames are summed and the demodulator finds out which wins.
func TestCaptureEffectEmerges(t *testing.T) {
	n := dsp.SamplesPerSymbol(sf)
	const weak, strong = 17, 200

	for _, tc := range []struct {
		name       string
		deltaDB    float64
		wantWinner int
	}{
		{"strong wins by 6 dB", 6, strong},
		{"strong wins by 3 dB", 3, strong},
		{"weak wins when it is the louder one", -6, weak},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := demod(Observe([]Transmission{
				{Node: "weak", Samples: symbol(weak), GainDB: 0},
				{Node: "strong", Samples: symbol(strong), GainDB: tc.deltaDB},
			}, Receiver{NoisePowerLinear: 0.001, Seed: 4417}, n))

			if got != tc.wantWinner {
				t.Errorf("captured symbol %d, expected %d to win", got, tc.wantWinner)
			}
		})
	}
}

// Equal-power collisions have no winner — the demodulator should be unsure.
// This is the case a packet model gets wrong in both directions.
func TestEqualPowerCollisionIsAmbiguous(t *testing.T) {
	n := dsp.SamplesPerSymbol(sf)
	_, confAlone := demod(Observe([]Transmission{
		{Samples: symbol(17), GainDB: 0},
	}, Receiver{NoisePowerLinear: 0.001, Seed: 1}, n))

	_, confCollide := demod(Observe([]Transmission{
		{Samples: symbol(17), GainDB: 0},
		{Samples: symbol(200), GainDB: 0},
	}, Receiver{NoisePowerLinear: 0.001, Seed: 1}, n))

	if confCollide >= confAlone {
		t.Errorf("collision confidence %.2f not below clean confidence %.2f", confCollide, confAlone)
	}
	if confCollide > 2.0 {
		t.Errorf("equal-power collision still looks confident (%.2f) — capture is being invented", confCollide)
	}
}

// A partial overlap must leave the un-collided part recoverable. This is the
// distinction that decides whether a clipped frame passes CRC, and it is exactly
// what "both frames fail" throws away.
func TestPartialOverlapIsPartial(t *testing.T) {
	n := dsp.SamplesPerSymbol(sf)
	window := n * 3
	obs := Observe([]Transmission{
		{Samples: dsp.Modulator{SF: sf}.Modulate([]int{10, 20, 30}), GainDB: 0, StartSample: 0},
		// interferer arrives late, hitting only the third symbol
		{Samples: symbol(200), GainDB: 6, StartSample: n * 2},
	}, Receiver{NoisePowerLinear: 0.001, Seed: 7}, window)

	got := dsp.Demodulator{SF: sf}.Demodulate(obs)
	if got[0] != 10 || got[1] != 20 {
		t.Errorf("clean symbols corrupted: got %v, want first two 10, 20", got[:2])
	}
	if got[2] == 30 {
		t.Errorf("third symbol survived a 6 dB interferer — collision not modelled")
	}
}

// Gain is applied as amplitude, not power: a 6 dB gain must double amplitude and
// quadruple power. Getting this wrong is a silent factor-of-two in every link.
func TestGainIsAmplitudeNotPower(t *testing.T) {
	n := dsp.SamplesPerSymbol(sf)
	base := dsp.SignalPower(Observe([]Transmission{{Samples: symbol(5), GainDB: 0}},
		Receiver{}, n))
	up6 := dsp.SignalPower(Observe([]Transmission{{Samples: symbol(5), GainDB: 6}},
		Receiver{}, n))
	ratio := up6 / base
	if math.Abs(ratio-4) > 0.1 {
		t.Errorf("+6 dB changed power by %.3fx, want 4x", ratio)
	}
}

// The channel is deterministic given a seed, or no result is reproducible.
func TestChannelDeterminism(t *testing.T) {
	n := dsp.SamplesPerSymbol(sf)
	mk := func() []complex128 {
		return Observe([]Transmission{{Samples: symbol(42), GainDB: -3, DelaySamples: 0.37}},
			Receiver{NoisePowerLinear: 0.01, Seed: 4417, Offset: 99}, n)
	}
	a, b := mk(), mk()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("same seed diverged at sample %d", i)
		}
	}
}
