package gpu

import (
	"fmt"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/dsp"
)

// The demod kernel against its CPU oracle, on real modulated symbols with
// real noise: the bins must agree wherever the decision is not a genuine
// near-tie. f32 against f64 can split a tie the noise made - which is why
// the verdict path keeps the CPU as the judge and the kernel is an
// accelerator, never an authority (ADR-0004, and the W6 gate).
func TestDemodBatchMatchesCPU(t *testing.T) {
	d, err := Open()
	if err != nil {
		t.Skipf("no GPU available: %v", err)
	}
	defer d.Close()
	t.Logf("GPU: %s (%s)", d.Name, d.Backend)

	for _, sf := range []int{7, 9, 11} {
		n := dsp.SamplesPerSymbol(sf)
		m := dsp.Modulator{SF: sf}
		syms := make([]int, 64)
		for i := range syms {
			syms[i] = (i * 41) % n
		}
		rx := m.Modulate(syms)
		sig := dsp.SignalPower(m.ModulateSymbol(0))
		noise := dsp.NoisePowerForSNR(sig, dsp.RequiredSNRdB[sf]+6)
		dsp.Philox{Seed: 33}.AddAWGN(rx, noise, 0)

		rx32 := make([]complex64, len(rx))
		for i, v := range rx {
			rx32[i] = complex64(v)
		}
		bins, conf, err := d.DemodBatch(rx32, sf)
		if err != nil {
			t.Fatalf("SF%d: %v", sf, err)
		}

		cpu := dsp.Demodulator{SF: sf}
		scratch := make([]complex128, n)
		disagreements := 0
		for i := range syms {
			want, wantConf := cpu.DemodulateSymbolInto(scratch, rx[i*n:(i+1)*n])
			if bins[i] != want {
				// Only a genuine near-tie may split: anything else is a
				// wrong kernel wearing precision as an excuse.
				if wantConf > 1.2 {
					t.Fatalf("SF%d symbol %d: gpu bin %d, cpu bin %d at confidence %.2f",
						sf, i, bins[i], want, wantConf)
				}
				disagreements++
			}
			_ = conf
		}
		if disagreements > len(syms)/8 {
			t.Fatalf("SF%d: %d of %d symbols split - more than ties explain",
				sf, disagreements, len(syms))
		}
	}
}

// SF12 must refuse loudly rather than truncate its FFT.
func TestDemodBatchRefusesSF12(t *testing.T) {
	d, err := Open()
	if err != nil {
		t.Skipf("no GPU available: %v", err)
	}
	defer d.Close()
	if _, _, err := d.DemodBatch(make([]complex64, 4096), 12); err == nil {
		t.Fatal("SF12 should refuse; the symbol does not fit workgroup memory")
	}
}

// The number the plan wants recorded: batch demodulation, CPU against GPU.
func BenchmarkDemodBatch(b *testing.B) {
	d, err := Open()
	if err != nil {
		b.Skipf("no GPU available: %v", err)
	}
	defer d.Close()

	const sf, count = 9, 512
	n := dsp.SamplesPerSymbol(sf)
	m := dsp.Modulator{SF: sf}
	syms := make([]int, count)
	for i := range syms {
		syms[i] = (i * 97) % n
	}
	rx := m.Modulate(syms)
	dsp.Philox{Seed: 4}.AddAWGN(rx, 0.5, 0)
	rx32 := make([]complex64, len(rx))
	for i, v := range rx {
		rx32[i] = complex64(v)
	}

	b.Run(fmt.Sprintf("cpu-sf%d-%dsyms", sf, count), func(b *testing.B) {
		cpu := dsp.Demodulator{SF: sf}
		scratch := make([]complex128, n)
		for i := 0; i < b.N; i++ {
			for s := 0; s < count; s++ {
				cpu.DemodulateSymbolInto(scratch, rx[s*n:(s+1)*n])
			}
		}
	})
	b.Run(fmt.Sprintf("gpu-sf%d-%dsyms", sf, count), func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, _, err := d.DemodBatch(rx32, sf); err != nil {
				b.Fatal(err)
			}
		}
	})
}
