package sdr

import (
	"math"
	"testing"
)

// The floor must land where the level control anchors it: a silent source
// with a stated PSD comes out as noise a few counts wide around 127, not
// silence and not clipping.
func TestFloorSitsAtItsAnchor(t *testing.T) {
	const psd, rate = 1e-17, 250000.0
	fl := newFloorNoise(psd)
	lvl := newLevelControl()
	iq := make([]complex128, 8192)
	fl.add(iq, rate)
	out := make([]byte, len(iq)*2)
	lvl.apply(iq, out, fl.sigma(rate))
	var sum, sumSq float64
	for _, b := range out {
		d := float64(b) - 127.5
		sum += d
		sumSq += d * d
	}
	mean := sum / float64(len(out))
	rms := math.Sqrt(sumSq / float64(len(out)))
	if math.Abs(mean) > 0.5 {
		t.Fatalf("floor is off-centre: mean %.2f counts", mean)
	}
	if rms < 1.5 || rms > 4 {
		t.Fatalf("floor RMS %.2f counts; want ~%.1f", rms, floorCounts)
	}
}

// A burst 60 dB over the floor must not clip - clipped chirps painted the
// whole client span - and the floor must come back afterwards.
func TestLevelControlNeverClipsAndRecovers(t *testing.T) {
	const psd, rate = 1e-17, 250000.0
	fl := newFloorNoise(psd)
	lvl := newLevelControl()
	sigma := fl.sigma(rate)
	burstAmp := sigma * 1000 // 60 dB up

	chunk := func(withBurst bool) []byte {
		iq := make([]complex128, 4096)
		if withBurst {
			for i := range iq {
				ph := 2 * math.Pi * 0.01 * float64(i)
				iq[i] = complex(burstAmp*math.Cos(ph), burstAmp*math.Sin(ph))
			}
		}
		fl.add(iq, rate)
		out := make([]byte, len(iq)*2)
		lvl.apply(iq, out, sigma)
		return out
	}

	burst := chunk(true)
	for i, b := range burst {
		if b == 0 || b == 255 {
			t.Fatalf("sample %d clipped to %d during the burst", i, b)
		}
	}
	// Quiet again: the scale glides back to the anchor within seconds of
	// stream (each chunk is ~16 ms here; 1.05 per chunk).
	var rms float64
	for i := 0; i < 400; i++ {
		out := chunk(false)
		var sumSq float64
		for _, b := range out {
			d := float64(b) - 127.5
			sumSq += d * d
		}
		rms = math.Sqrt(sumSq / float64(len(out)))
	}
	if rms < 1.5 {
		t.Fatalf("floor never recovered after the burst: RMS %.2f counts", rms)
	}
}
