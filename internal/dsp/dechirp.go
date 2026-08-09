package dsp

// DechirpCPU is the CPU reference for shaders/dechirp.wgsl.
//
// Per ADR-0004 this is the oracle, not a fallback: the GPU kernel is tested
// against it, because a wrong dechirp produces a plausible waterfall and
// slightly wrong sensitivity that nobody notices for months.
//
// Kept as an explicit separate function rather than reusing DemodulateSymbol's
// internals, so the thing being compared is exactly the thing the shader
// computes.
func DechirpCPU(rx []complex128, sf int) []complex128 {
	n := SamplesPerSymbol(sf)
	base := Modulator{SF: sf}.BaseUpchirp()
	out := make([]complex128, len(rx))
	for idx := range rx {
		i := idx % n
		b := base[i]
		out[idx] = rx[idx] * complex(real(b), -imag(b)) // multiply by the conjugate
	}
	return out
}
