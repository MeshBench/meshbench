package dsp

import (
	"fmt"
	"testing"
)

// MSIM-3: measure the CPU ceiling. The question is not "is the GPU faster" but
// "at what network size does the CPU stop keeping up with real time", because
// that is the point where ADR-0004's GPU work stops being optional.
func TestCPUCeiling(t *testing.T) {
	if testing.Short() {
		t.Skip("benchmark sweep; run without -short")
	}
	fmt.Printf("\n  SF   symbols/s   real-time receivers @125kHz   µs/symbol\n")
	fmt.Printf("  ----------------------------------------------------------\n")
	for _, sf := range []int{7, 9, 10, 12} {
		m, d := Modulator{SF: sf}, Demodulator{SF: sf}
		wave := m.ModulateSymbol(42)

		// Time enough symbols to get past timer noise.
		iters := 20000
		if sf >= 10 {
			iters = 4000
		}
		if sf >= 12 {
			iters = 1000
		}
		start := testing.Benchmark(func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < iters; i++ {
				d.DemodulateSymbol(wave)
			}
		})
		_ = start

		t0 := nowNanos()
		for i := 0; i < iters; i++ {
			d.DemodulateSymbol(wave)
		}
		elapsed := float64(nowNanos()-t0) / 1e9

		symsPerSec := float64(iters) / elapsed
		// Symbol rate on air at BW 125 kHz: BW / 2^SF symbols per second.
		onAir := 125000.0 / float64(SamplesPerSymbol(sf))
		receivers := symsPerSec / onAir
		fmt.Printf("  SF%-2d  %9.0f   %27.0f   %8.1f\n",
			sf, symsPerSec, receivers, 1e6/symsPerSec)
	}
}

func nowNanos() int64 { return timeNow() }
