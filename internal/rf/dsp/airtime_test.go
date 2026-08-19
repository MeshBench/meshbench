package dsp

import (
	"math"
	"testing"
)

// Reference values from the Semtech LoRa airtime calculation, with MeshCore's
// own preamble lengths (32 symbols at SF7/8, 16 above) and explicit header plus
// CRC, which is how MeshCore configures the radio.
//
// These are not regression values captured from this implementation — that
// would test nothing. They are computed from the published formula, which is
// the only thing that makes a mismatch meaningful.
func TestAirtimeMatchesTheFirmware(t *testing.T) {
	const bw = 125_000.0
	cases := []struct {
		sf, n int
		want  float64 // ms
	}{
		// SF7, 32-symbol preamble: (32+4.25+8) symbols of overhead at 1.024 ms.
		{7, 16, 76},
		{7, 64, 142},
		// SF9, 16-symbol preamble at 4.096 ms/symbol.
		{9, 16, 197},
		// SF12 crosses the 16 ms low-data-rate threshold, so sfDivisor drops to
		// 4*(sf-2) — the branch most likely to be wrong.
		{12, 16, 1581},
		{12, 64, 3055},
	}
	for _, c := range cases {
		got := AirtimeMillis(c.sf, bw, 1, c.n, true, true)
		if math.Abs(got-c.want) > 1 {
			t.Errorf("SF%d, %d bytes: got %.0f ms, want %.0f ms", c.sf, c.n, got, c.want)
		}
	}
}

// Airtime must rise with every parameter that costs time. A formula that gets
// the absolute numbers right but the monotonicity wrong will still produce a
// mesh whose CSMA behaves nothing like the real one.
func TestAirtimeIsMonotonic(t *testing.T) {
	const bw = 125_000.0
	prev := 0.0
	for sf := 7; sf <= 12; sf++ {
		got := AirtimeMillis(sf, bw, 1, 32, true, true)
		if got <= prev {
			t.Errorf("SF%d airtime %.0f ms did not exceed SF%d's %.0f ms", sf, got, sf-1, prev)
		}
		prev = got
	}
	for n := 8; n < 200; n += 8 {
		if AirtimeMillis(9, bw, 1, n+8, true, true) < AirtimeMillis(9, bw, 1, n, true, true) {
			t.Fatalf("airtime fell between %d and %d bytes", n, n+8)
		}
	}
	// A weaker coding rate is more redundancy, so more time on air.
	if AirtimeMillis(9, bw, 4, 32, true, true) <= AirtimeMillis(9, bw, 1, 32, true, true) {
		t.Error("4/8 coding should take longer than 4/5")
	}
	// Halving the bandwidth doubles every symbol.
	full := AirtimeMillis(9, bw, 1, 32, true, true)
	half := AirtimeMillis(9, bw/2, 1, 32, true, true)
	if r := half / full; r < 1.95 || r > 2.05 {
		t.Errorf("halving bandwidth changed airtime by %.2fx, want ~2x", r)
	}
}

func TestPreambleFollowsMeshCore(t *testing.T) {
	for sf, want := range map[int]int{7: 32, 8: 32, 9: 16, 12: 16} {
		if got := PreambleSymbols(sf); got != want {
			t.Errorf("SF%d preamble %d, want %d", sf, got, want)
		}
	}
}
