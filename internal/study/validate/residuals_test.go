package validate_test

import (
	"math"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/study/validate"
	"github.com/MeshBench/meshbench/internal/world/provider"
)

type flat struct{ h float64 }

func (f flat) ElevationM(_, _ float64) (float64, bool) { return f.h, true }

type noTerrain struct{}

func (noTerrain) ElevationM(_, _ float64) (float64, bool) { return 0, false }

// omni is a stacked vertical of the given gain, pointing nowhere in
// particular, which is what most of a real network is on.
func omni(gainDBi float64) antenna.Mounted {
	return antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: gainDBi}}
}

func stations() map[string]validate.Station {
	return map[string]validate.Station{
		"origin": {Name: "origin", Lat: 56.70, Lon: -3.90, HeightAGLm: 20, TxPowerDBm: 22, Antenna: omni(3), NoiseFigureDB: 6},
		"rx-a":   {Name: "rx-a", Lat: 56.75, Lon: -3.80, HeightAGLm: 10, TxPowerDBm: 22, Antenna: omni(2), NoiseFigureDB: 6},
		"rx-b":   {Name: "rx-b", Lat: 56.60, Lon: -4.00, HeightAGLm: 8, TxPowerDBm: 22, Antenna: omni(2), NoiseFigureDB: 6},
	}
}

func params() validate.Params {
	return validate.Params{FreqMHz: 869.525, SF: 10, BandwidthHz: 250_000, ProfileStepM: 100}
}

func rx(receiver, packet string, snr float64) provider.Reception {
	return provider.Reception{
		Receiver: receiver, PacketID: packet, Origin: "origin",
		HasSNR: true, SNRdB: snr, Source: "test",
	}
}

// The whole point of the package: given observations, say whether the model is
// optimistic and by how much. A residual is observed minus predicted, so a
// negative mean means the model claimed more signal than arrived.
func TestBiasDirectionIsUnambiguous(t *testing.T) {
	st := stations()
	// Compute what the model predicts, then feed back observations that are a
	// known amount worse. The bias must come out as exactly that amount.
	base, err := validate.Compare([]provider.Reception{rx("rx-a", "p1", 0)}, st, flat{100}, params())
	if err != nil {
		t.Fatal(err)
	}
	if base.Used != 1 {
		t.Fatalf("expected one usable observation, got %d", base.Used)
	}
	predicted := base.Residuals[0].PredictedSNRdB

	const worseBy = 7.0
	rep, err := validate.Compare([]provider.Reception{rx("rx-a", "p1", predicted-worseBy)}, st, flat{100}, params())
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(rep.MeanDB+worseBy) > 0.01 {
		t.Errorf("bias %+.2f dB, want %+.2f", rep.MeanDB, -worseBy)
	}
	if !contains(rep.Verdict(), "OPTIMISTIC") {
		t.Errorf("a model predicting 7 dB more than arrived was not called optimistic:\n%s", rep.Verdict())
	}

	better, err := validate.Compare([]provider.Reception{rx("rx-a", "p1", predicted+4)}, st, flat{100}, params())
	if err != nil {
		t.Fatal(err)
	}
	if !contains(better.Verdict(), "pessimistic") {
		t.Errorf("a model predicting 4 dB less than arrived was not called pessimistic:\n%s", better.Verdict())
	}
}

// The rule the package exists to enforce. A node that did not report a packet
// is not evidence it could not hear one — it may not be an observer, it may
// have been transmitting, it may have been off. Counting silence as a negative
// observation manufactures a confident, wrong calibration.
func TestSilenceIsCountedButNeverUsed(t *testing.T) {
	st := stations()
	// Only rx-a reports; rx-b is silent.
	rep, err := validate.Compare([]provider.Reception{rx("rx-a", "p1", -5)}, st, flat{100}, params())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Used != 1 {
		t.Errorf("used %d observations, want 1 — silence leaked into the residuals", rep.Used)
	}
	if rep.SilentReceivers != 1 {
		t.Errorf("counted %d silent receivers, want 1", rep.SilentReceivers)
	}
	if !contains(rep.Verdict(), "not evidence") {
		t.Errorf("the verdict does not explain what silence means:\n%s", rep.Verdict())
	}
}

// A reception from a node placed at 5 km cannot constrain a model to a decibel.
// Including it would let the loosest data set the calibration.
func TestUncertainPositionsAreExcluded(t *testing.T) {
	st := stations()
	loose := st["rx-b"]
	loose.UncertaintyKm = 5
	st["rx-b"] = loose

	rep, err := validate.Compare([]provider.Reception{
		rx("rx-a", "p1", -5),
		rx("rx-b", "p1", -9),
	}, st, flat{100}, params())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Used != 1 {
		t.Errorf("used %d observations; the 5 km position should have been excluded", rep.Used)
	}
	if rep.SkippedUncertain != 1 {
		t.Errorf("skipped-uncertain is %d, want 1", rep.SkippedUncertain)
	}
}

// A run that silently discards most of its input looks exactly like one that
// did not, so every exclusion is counted and reported.
func TestExclusionsAreAllAccountedFor(t *testing.T) {
	st := stations()
	obs := []provider.Reception{
		rx("rx-a", "p1", -5),
		{Receiver: "rx-b", PacketID: "p1", Origin: "origin"},                  // no SNR
		{Receiver: "unknown", PacketID: "p1", Origin: "origin", HasSNR: true}, // no position
	}
	rep, err := validate.Compare(obs, st, flat{100}, params())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Used != 1 || rep.SkippedNoSNR != 1 || rep.SkippedNoPosition != 1 {
		t.Errorf("accounting is wrong: used=%d noSNR=%d noPos=%d",
			rep.Used, rep.SkippedNoSNR, rep.SkippedNoPosition)
	}
	total := rep.Used + rep.SkippedNoSNR + rep.SkippedNoPosition + rep.SkippedUncertain + rep.SkippedNoTerrain
	if total != len(obs) {
		t.Errorf("%d observations in, %d accounted for", len(obs), total)
	}
}

// Spread matters more than bias: a constant bias can be calibrated out and
// scatter cannot. The verdict has to say so, because a big bias with tight
// scatter looks worse at a glance and is in much better shape.
func TestSpreadIsReportedSeparatelyFromBias(t *testing.T) {
	st := stations()
	base, _ := validate.Compare([]provider.Reception{rx("rx-a", "p1", 0)}, st, flat{100}, params())
	predicted := base.Residuals[0].PredictedSNRdB

	var obs []provider.Reception
	for i, delta := range []float64{-12, -6, 0, 6, 12} {
		obs = append(obs, rx("rx-a", packetID(i), predicted+delta))
	}
	rep, err := validate.Compare(obs, st, flat{100}, params())
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(rep.MeanDB) > 0.01 {
		t.Errorf("symmetric scatter gave a bias of %+.2f dB", rep.MeanDB)
	}
	if rep.StdDevDB < 8 {
		t.Errorf("standard deviation %.1f dB does not reflect a +/-12 dB spread", rep.StdDevDB)
	}
	if rep.P10DB > -9 || rep.P90DB < 9 {
		t.Errorf("percentiles %+.1f to %+.1f do not bracket the spread", rep.P10DB, rep.P90DB)
	}
	if !contains(rep.Verdict(), "Bias can be calibrated out; spread cannot") {
		t.Error("the verdict does not distinguish bias from scatter")
	}
}

// One reception has a bias and no spread. Reporting "spread 0.0 dB standard
// deviation" for it claims the tightest calibration anybody has ever measured,
// off a number compared with itself.
func TestASingleResidualClaimsNoSpread(t *testing.T) {
	rep, err := validate.Compare([]provider.Reception{rx("rx-a", "p1", -5)},
		stations(), flat{100}, params())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Used != 1 {
		t.Fatalf("used %d observations, want 1", rep.Used)
	}
	v := rep.Verdict()
	if contains(v, "standard deviation") {
		t.Errorf("one observation was given a standard deviation:\n%s", v)
	}
	if contains(v, "percentile") {
		t.Errorf("one observation was given a percentile range:\n%s", v)
	}
	if !contains(v, "spread unknown") {
		t.Errorf("the verdict does not say the spread is unknown:\n%s", v)
	}
	// Two of them do have a spread between them, and it is still reported.
	both, err := validate.Compare([]provider.Reception{
		rx("rx-a", "p1", -5), rx("rx-b", "p1", -12)}, stations(), flat{100}, params())
	if err != nil {
		t.Fatal(err)
	}
	if !contains(both.Verdict(), "standard deviation") {
		t.Errorf("two observations lost their spread:\n%s", both.Verdict())
	}
}

func TestNoTerrainIsCountedNotFatal(t *testing.T) {
	rep, err := validate.Compare([]provider.Reception{rx("rx-a", "p1", -5)}, stations(), noTerrain{}, params())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Used != 0 || rep.SkippedNoTerrain != 1 {
		t.Errorf("used=%d noTerrain=%d", rep.Used, rep.SkippedNoTerrain)
	}
	if !contains(rep.Verdict(), "Nothing here says anything about the model") {
		t.Errorf("an empty result should say so plainly:\n%s", rep.Verdict())
	}
}

func TestRejectsUnusableParameters(t *testing.T) {
	for name, p := range map[string]validate.Params{
		"no frequency": {SF: 10},
		"bad SF":       {FreqMHz: 869.525, SF: 3},
	} {
		if _, err := validate.Compare(nil, stations(), flat{100}, p); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func packetID(i int) string { return string(rune('a' + i)) }

func contains(s, sub string) bool { return strings.Contains(s, sub) }
