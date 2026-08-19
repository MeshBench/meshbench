package dsp

import "testing"

// The identity ModulateSymbol relies on. x_s[i] = chirpSample(i, s, n) uses
// k = (s+i) mod n, and the base chirp is the same expression with k = j — so a
// modulated symbol is exactly the base chirp rotated by s.
//
// Asserted against the trigonometric definition directly, not against the
// rotation, because the whole point is that the fast path and the slow one
// agree bit for bit.
func TestModulationIsACyclicShift(t *testing.T) {
	for _, sf := range []int{7, 9, 12} {
		n := SamplesPerSymbol(sf)
		m := Modulator{SF: sf}
		for _, s := range []int{0, 1, 7, n / 3, n - 1} {
			got := m.ModulateSymbol(s)
			for i := 0; i < n; i++ {
				want := chirpSample(i, s, n)
				if got[i] != want {
					t.Fatalf("SF%d symbol %d sample %d: rotation gave %v, definition gives %v",
						sf, s, i, got[i], want)
				}
			}
		}
	}
}

// BaseUpchirp hands out copies. A caller that mutates what it is given must not
// corrupt the cache every other symbol is built from.
func TestBaseUpchirpIsNotShared(t *testing.T) {
	a := Modulator{SF: 7}.BaseUpchirp()
	a[0] = 12345
	b := Modulator{SF: 7}.BaseUpchirp()
	if b[0] == 12345 {
		t.Fatal("mutating a returned chirp corrupted the shared one")
	}
	// And the modulator still produces the true waveform.
	if got := (Modulator{SF: 7}).ModulateSymbol(0); got[0] != chirpSample(0, 0, SamplesPerSymbol(7)) {
		t.Fatal("the cache was poisoned")
	}
}

func BenchmarkModulateSymbol(b *testing.B) {
	m := Modulator{SF: 12}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.ModulateSymbol(i % 4096)
	}
}

func BenchmarkDemodulateSymbol(b *testing.B) {
	m, d := Modulator{SF: 12}, Demodulator{SF: 12}
	rx := m.ModulateSymbol(1234)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.DemodulateSymbol(rx)
	}
}
