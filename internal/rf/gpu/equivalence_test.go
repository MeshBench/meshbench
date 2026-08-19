package gpu

import (
	"fmt"
	"math"
	"math/cmplx"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/dsp"
)

// The harness ADR-0004 requires: every GPU kernel has a CPU twin, and a test
// asserts they agree. Skipped when no GPU is present, which is the normal case
// in CI and on the dev VM — that is exactly why the CPU path is the oracle.
func TestDechirpMatchesCPU(t *testing.T) {
	d, err := Open()
	if err != nil {
		t.Skipf("no GPU available: %v", err)
	}
	defer d.Close()
	t.Logf("GPU: %s (%s)", d.Name, d.Backend)

	for _, sf := range []int{7, 9, 10} {
		n := dsp.SamplesPerSymbol(sf)
		// A batch of real modulated symbols, not random noise — the kernel must
		// be right on the input it will actually see.
		var rx64 []complex128
		m := dsp.Modulator{SF: sf}
		for _, s := range []int{0, 1, n / 3, n - 1} {
			rx64 = append(rx64, m.ModulateSymbol(s)...)
		}
		rx32 := make([]complex64, len(rx64))
		for i, v := range rx64 {
			rx32[i] = complex64(v)
		}

		want := dsp.DechirpCPU(rx64, sf)
		got, err := d.Dechirp(rx32, sf)
		if err != nil {
			t.Fatalf("SF%d: %v", sf, err)
		}
		if len(got) != len(want) {
			t.Fatalf("SF%d: got %d samples, want %d", sf, len(got), len(want))
		}

		var maxErr float64
		for i := range want {
			e := cmplx.Abs(complex128(got[i]) - want[i])
			if e > maxErr {
				maxErr = e
			}
		}
		// f32 on the GPU vs f64 on the CPU: agreement within tolerance is the
		// contract, not bit-identity. ADR-0004 says so explicitly, because
		// floating-point associativity differs between backends.
		const tol = 1e-3
		if maxErr > tol {
			t.Errorf("SF%d: max deviation %.3e exceeds %.0e", sf, maxErr, tol)
		}
		fmt.Printf("  SF%-2d  %5d samples   max deviation %.3e\n", sf, len(want), maxErr)
	}
}

// The dechirped peak must land on the symbol that was sent — the property the
// whole demodulator depends on, checked through the GPU path.
func TestGPUDechirpPreservesSymbol(t *testing.T) {
	d, err := Open()
	if err != nil {
		t.Skipf("no GPU available: %v", err)
	}
	defer d.Close()

	const sf = 9
	n := dsp.SamplesPerSymbol(sf)
	for _, want := range []int{0, 42, 300, n - 1} {
		wave := dsp.Modulator{SF: sf}.ModulateSymbol(want)
		rx32 := make([]complex64, n)
		for i, v := range wave {
			rx32[i] = complex64(v)
		}
		got, err := d.Dechirp(rx32, sf)
		if err != nil {
			t.Fatal(err)
		}
		buf := make([]complex128, n)
		for i, v := range got {
			buf[i] = complex128(v)
		}
		dsp.FFT(buf)
		peak, peakIdx := 0.0, 0
		for i, v := range buf {
			if mag := real(v)*real(v) + imag(v)*imag(v); mag > peak {
				peak, peakIdx = mag, i
			}
		}
		if peakIdx != want {
			t.Errorf("GPU dechirp + FFT recovered symbol %d, want %d", peakIdx, want)
		}
	}
}

func TestToleranceIsJustified(t *testing.T) {
	// f32 has ~7 decimal digits; over a 2^10 symbol the accumulated phase error
	// is well under 1e-3. If this ever needs loosening, that is a finding about
	// the kernel, not a licence to widen the band.
	if eps := math.Nextafter32(1, 2) - 1; float64(eps) > 1.2e-7 {
		t.Errorf("f32 epsilon %.2e larger than assumed", eps)
	}
}
