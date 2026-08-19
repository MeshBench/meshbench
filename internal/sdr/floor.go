// The front end's own noise, painted across the client's whole span.
//
// A real dongle's floor fills its sample rate - its ADC and tuner make
// noise everywhere, not just in the channel someone cares about. The
// simulation's receivers carry noise only inside their bandwidth, so a
// resampled stream showed a synthetic-looking silence either side of the
// channel. The server adds the floor itself, at the receiver's own noise
// density, so what the client sees and what the verdicts hear are the same
// claim about the same floor.
package sdr

import (
	"math"
	"math/rand"
)

type floorNoise struct {
	psd float64
	rng *rand.Rand
}

func newFloorNoise(psd float64) *floorNoise {
	// A fixed seed: deterministic per connection, never shared.
	return &floorNoise{psd: psd, rng: rand.New(rand.NewSource(0x5DEECE66D))}
}

// sigma is the per-component amplitude of the floor at a given rate: total
// power psd*rate, split between I and Q.
func (f *floorNoise) sigma(rateHz float64) float64 {
	if f.psd <= 0 {
		return 0
	}
	return math.Sqrt(f.psd * rateHz / 2)
}

func (f *floorNoise) add(iq []complex128, rateHz float64) {
	sig := f.sigma(rateHz)
	if sig == 0 {
		return
	}
	for i := range iq {
		iq[i] += complex(f.rng.NormFloat64()*sig, f.rng.NormFloat64()*sig)
	}
}
