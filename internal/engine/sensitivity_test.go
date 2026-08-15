package engine

import "testing"

// The package's other fakes live in the external test package, which this file
// is not: the counters are internal and are the thing being tested.
type flatEarth struct{}

func (flatEarth) ElevationM(_, _ float64) (float64, bool) { return 0, true }

func TestABandCountsEveryReceptionCloserThanIt(t *testing.T) {
	var s sensitivity
	s.note(0.5, true) // inside every band
	s.note(2.5, true) // inside 3, 6, 10
	s.note(20, true)  // inside none
	got := s.read()

	if got.Decoded != 3 {
		t.Fatalf("decoded = %d, want 3", got.Decoded)
	}
	// MarginEdgesDB is {1, 2, 3, 6, 10}.
	for i, want := range []int{1, 1, 2, 2, 2} {
		if got.LostIfWorseBy[i] != want {
			t.Errorf("within %.0f dB: got %d, want %d", MarginEdgesDB[i], got.LostIfWorseBy[i], want)
		}
	}
}

func TestAMissIsCountedByHowFarShortItFell(t *testing.T) {
	var s sensitivity
	s.note(-0.5, false)
	s.note(-4, false)
	got := s.read()

	if got.Missed != 2 || got.Decoded != 0 {
		t.Fatalf("missed = %d, decoded = %d, want 2 and 0", got.Missed, got.Decoded)
	}
	for i, want := range []int{1, 1, 1, 2, 2} {
		if got.WonIfBetterBy[i] != want {
			t.Errorf("within %.0f dB: got %d, want %d", MarginEdgesDB[i], got.WonIfBetterBy[i], want)
		}
	}
}

// The figure a receiver study is after: what share of what arrived would not
// have, had the receiver been a little worse.
func TestAtRiskIsAShareOfWhatDecoded(t *testing.T) {
	var s sensitivity
	for i := 0; i < 3; i++ {
		s.note(0.5, true)
	}
	for i := 0; i < 7; i++ {
		s.note(30, true)
	}
	got := s.read()
	if r := got.AtRisk(1); r < 0.29 || r > 0.31 {
		t.Fatalf("AtRisk(2 dB) = %.3f, want 0.30", r)
	}
	if r := got.AtRisk(99); r != 0 {
		t.Fatalf("an index past the bands must not panic or invent a figure, got %v", r)
	}
}

func TestNothingHeardIsNotADivisionByZero(t *testing.T) {
	var s sensitivity
	if r := s.read().AtRisk(0); r != 0 {
		t.Fatalf("AtRisk with no receptions = %v, want 0", r)
	}
}

func TestResetLetsOneCellReportItsOwnReceptions(t *testing.T) {
	e := New(flatEarth{}, Config{StepMs: 10})
	e.sens.note(0.5, true)
	e.ResetSensitivity()
	if got := e.Sensitivity(); got.Decoded != 0 || got.LostIfWorseBy[0] != 0 {
		t.Fatalf("reset left %+v", got)
	}
}
