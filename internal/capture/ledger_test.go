package capture

import "testing"

// The five outcomes must stay distinct — collapsing any two of them is the
// packet-model failure this whole package exists to avoid.
func TestClassifyDistinguishesAllFiveOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name                              string
		offered, demod, crc, saw, relayed bool
		want                              Outcome
	}{
		{"nothing arrived", false, false, false, false, false, OutOfRange},
		{"too weak to demodulate", true, false, false, false, false, NotDemodulated},
		{"demodulated but corrupt", true, true, false, false, false, CRCFailed},
		{"received and dropped by dedup", true, true, true, true, false, DroppedByFirmware},
		{"received and relayed", true, true, true, true, true, Relayed},
	} {
		if got := Classify(tc.offered, tc.demod, tc.crc, tc.saw, tc.relayed); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A frame the radio recovered but the firmware never saw is a plumbing bug, and
// must not be silently reported as a clean receive.
func TestFirmwareNeverSawIsNotSuccess(t *testing.T) {
	if got := Classify(true, true, true, false, false); got == Relayed || got == Accepted {
		t.Errorf("frame the firmware never saw classified as success (%q)", got)
	}
}

func TestSummariseCountsTheInterestingCases(t *testing.T) {
	var l Ledger
	rows := []struct {
		to             string
		outcome        Outcome
		offered, demod bool
	}{
		{"node-04", Relayed, true, true},
		{"node-07", DroppedByFirmware, true, true},
		{"node-09", CRCFailed, true, true},
		{"node-12", NotDemodulated, true, false},
		{"node-15", OutOfRange, false, false},
	}
	for _, r := range rows {
		l.Record(Reception{PacketID: 4471, ToNode: r.to, Outcome: r.outcome,
			Offered: r.offered, Demod: r.demod})
	}
	s := l.Summarise(4471)
	if s.Total != 5 || s.Offered != 4 || s.Demodulated != 3 || s.Accepted != 2 || s.Relayed != 1 {
		t.Errorf("summary = %+v; want total 5, offered 4, demodulated 3, accepted 2, relayed 1", s)
	}
	// The headline number a planner reads: reached 4 of 5, only 1 relayed.
	if len(l.ForPacket(4471)) != 5 {
		t.Errorf("ForPacket returned %d rows, want 5", len(l.ForPacket(4471)))
	}
}
