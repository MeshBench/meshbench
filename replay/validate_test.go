package replay_test

import (
	"strings"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/provider"
	"github.com/A13xB0/meshcoresim/internal/replay"
)

func rx(pkt, origin, recv string, snr float64) provider.Reception {
	return provider.Reception{
		PacketID: pkt, Origin: origin, Receiver: recv,
		HasSNR: true, SNRdB: snr, At: at(1),
	}
}

// The ADR-0015 table, as a test: agreement, an optimistic miss, and a
// pessimistic one, each landing in the right bucket.
func TestCompareBucketsTheVerdicts(t *testing.T) {
	obs := []provider.Reception{
		rx("p1", "alpha", "node-04", -6),  // model will say -4: agrees, +2 dB
		rx("p1", "alpha", "node-07", -12), // model says -2: 10 dB off
		rx("p2", "alpha", "node-09", 3),   // model says no decode: pessimistic
	}
	predict := func(origin, receiver string) (float64, bool, bool) {
		switch receiver {
		case "node-04":
			return -4, true, true
		case "node-07":
			return -2, true, true
		case "node-09":
			return -25, false, true
		case "node-12":
			// Online, heard nothing, model predicts a comfortable decode:
			// the row that is the whole point.
			return 8, true, true
		}
		return 0, false, false
	}
	rep := replay.Compare(obs, predict, []string{"node-04", "node-07", "node-09", "node-12"})

	if rep.Samples != 3 {
		t.Fatalf("samples = %d, want 3", rep.Samples)
	}
	// node-12 was online and heard neither packet while the model predicted a
	// comfortable decode for both — one optimistic row per silence.
	if rep.Optimistic != 2 || rep.Pessimistic != 1 {
		t.Fatalf("optimistic=%d pessimistic=%d, want 2 and 1", rep.Optimistic, rep.Pessimistic)
	}
	foundOptimistic := false
	for _, row := range rep.Rows {
		if row.Receiver == "node-12" && strings.Contains(row.Verdict, "OPTIMISTIC") {
			foundOptimistic = true
		}
	}
	if !foundOptimistic {
		t.Fatal("the model-optimistic silence was not reported")
	}
	if s := rep.Summary(); !strings.Contains(s, "3 comparisons") {
		t.Fatalf("summary = %q", s)
	}
}

func TestCalibrateRefusesSmallSamples(t *testing.T) {
	rep := replay.Report{Samples: 6, MeanBiasDB: 4.2}
	if _, err := replay.Calibrate(rep); err == nil {
		t.Fatal("six observations produced a calibration constant")
	}
	rep.Samples = 40
	c, err := replay.Calibrate(rep)
	if err != nil {
		t.Fatal(err)
	}
	if c.ExcessLossDB != 4.2 {
		t.Fatalf("excess loss = %v, want the mean bias", c.ExcessLossDB)
	}
}

// An observer that was offline must not generate optimistic rows: absence of
// an observation is weak evidence, and only a listening node's silence counts.
func TestCompareIgnoresOfflineObservers(t *testing.T) {
	obs := []provider.Reception{rx("p1", "alpha", "node-04", -6)}
	predict := func(_, _ string) (float64, bool, bool) { return 10, true, true }
	rep := replay.Compare(obs, predict, []string{"node-04"}) // node-99 not listed
	if rep.Optimistic != 0 {
		t.Fatalf("optimistic = %d from an observer nobody said was online", rep.Optimistic)
	}
}
