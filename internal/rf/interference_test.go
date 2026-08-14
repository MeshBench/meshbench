package rf_test

import (
	"math"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf"
)

func channel() rf.Channel { return rf.Channel{CentreHz: 869.525e6, BandwidthHz: 250e3} }

// Bandwidth matters as much as power. A wideband emitter puts only the fraction
// of its energy that lands in our channel into our receiver, and treating all
// of it as in-channel overstates the harm by orders of magnitude.
func TestWidebandEmitterOnlyContributesItsOverlap(t *testing.T) {
	wide := rf.Emitter{
		Name: "paging", CentreHz: 869.525e6, BandwidthHz: 25e6,
		ERPdBm: 57, DutyCycle: 0.05, Kind: "paging",
	}
	narrow := rf.Emitter{
		Name: "telemetry", CentreHz: 869.525e6, BandwidthHz: 25e3,
		ERPdBm: 57, DutyCycle: 0.05, Kind: "telemetry",
	}

	got, err := rf.Interference([]rf.Emitter{wide, narrow},
		map[string]float64{"paging": 100, "telemetry": 100}, channel())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]rf.InterferenceAt{}
	for _, g := range got {
		byName[g.Emitter] = g
	}

	// The narrow one sits entirely inside our channel, so all of its power
	// counts. The wide one spreads the same ERP over 100x the bandwidth.
	if f := byName["telemetry"].OverlapFraction; math.Abs(f-1) > 1e-9 {
		t.Errorf("a 25 kHz emitter inside a 250 kHz channel overlapped %.3f, want 1", f)
	}
	if f := byName["paging"].OverlapFraction; math.Abs(f-0.01) > 1e-6 {
		t.Errorf("a 25 MHz emitter overlapping 250 kHz gave %.4f, want 0.01", f)
	}
	if d := byName["telemetry"].InChannelDBm - byName["paging"].InChannelDBm; math.Abs(d-20) > 0.1 {
		t.Errorf("same ERP, 100x the bandwidth: %.1f dB apart, want 20", d)
	}
}

func TestOutOfBandEmitterContributesNothing(t *testing.T) {
	away := rf.Emitter{Name: "pmr", CentreHz: 446e6, BandwidthHz: 12.5e3, ERPdBm: 40, DutyCycle: 0.2}
	got, err := rf.Interference([]rf.Emitter{away}, map[string]float64{"pmr": 80}, channel())
	if err != nil {
		t.Fatal(err)
	}
	if got[0].OverlapFraction != 0 {
		t.Errorf("a 446 MHz emitter overlapped an 869 MHz channel by %.3f", got[0].OverlapFraction)
	}
	if !math.IsInf(got[0].InChannelDBm, -1) {
		t.Errorf("out-of-band power was %.1f dBm, want none at all", got[0].InChannelDBm)
	}
}

// The distinction the package exists for. An emitter with a 5% duty cycle does
// not raise a noise floor — it destroys one transmission in twenty. Folding it
// into an average makes a mesh look uniformly slightly worse, when what it
// actually does is fail intermittently, which is far harder to live with.
func TestBurstyInterferenceIsNotANoiseFloor(t *testing.T) {
	const thermal = -117.0
	bursty := rf.Emitter{Name: "paging", CentreHz: 869.525e6, BandwidthHz: 25e3, ERPdBm: 57, DutyCycle: 0.05}
	continuous := rf.Emitter{Name: "link", CentreHz: 869.525e6, BandwidthHz: 25e3, ERPdBm: 10, DutyCycle: 1.0}

	burstOnly, err := rf.Interference([]rf.Emitter{bursty}, map[string]float64{"paging": 150}, channel())
	if err != nil {
		t.Fatal(err)
	}
	if floor := rf.ElevatedNoiseFloorDBm(thermal, burstOnly); math.Abs(floor-thermal) > 0.01 {
		t.Errorf("a 5%% duty emitter raised the noise floor to %.2f dBm", floor)
	}
	if len(rf.Bursty(burstOnly, thermal)) != 1 {
		t.Error("the bursty source was not reported at all, so it would be invisible")
	}

	contOnly, err := rf.Interference([]rf.Emitter{continuous}, map[string]float64{"link": 110}, channel())
	if err != nil {
		t.Fatal(err)
	}
	floor := rf.ElevatedNoiseFloorDBm(thermal, contOnly)
	if floor <= thermal+0.5 {
		t.Errorf("a continuous source at %.1f dBm did not raise the %.1f dBm floor (got %.1f)",
			contOnly[0].InChannelDBm, thermal, floor)
	}
}

// Two sources add in power, not in decibels. Adding decibels is a mistake that
// produces a plausible number and is wrong by tens of dB.
func TestSourcesAddInPower(t *testing.T) {
	const thermal = -200.0 // negligible, so only the sources matter
	e := func(name string) rf.Emitter {
		return rf.Emitter{Name: name, CentreHz: 869.525e6, BandwidthHz: 25e3, ERPdBm: 0, DutyCycle: 1}
	}
	one, _ := rf.Interference([]rf.Emitter{e("a")}, map[string]float64{"a": 100}, channel())
	two, _ := rf.Interference([]rf.Emitter{e("a"), e("b")},
		map[string]float64{"a": 100, "b": 100}, channel())

	single := rf.ElevatedNoiseFloorDBm(thermal, one)
	double := rf.ElevatedNoiseFloorDBm(thermal, two)
	if d := double - single; math.Abs(d-3.0103) > 0.01 {
		t.Errorf("two equal sources differ from one by %.3f dB, want 3.01", d)
	}
}

func TestDescribeSeparatesTheTwoFailures(t *testing.T) {
	const thermal = -117.0
	sources, err := rf.Interference([]rf.Emitter{
		{Name: "paging", CentreHz: 869.525e6, BandwidthHz: 25e3, ERPdBm: 57, DutyCycle: 0.05},
		{Name: "link", CentreHz: 869.525e6, BandwidthHz: 25e3, ERPdBm: 10, DutyCycle: 1.0},
	}, map[string]float64{"paging": 150, "link": 110}, channel())
	if err != nil {
		t.Fatal(err)
	}
	text := rf.Describe(thermal, sources)
	if !strings.Contains(text, "raised") {
		t.Errorf("the continuous source is not described:\n%s", text)
	}
	if !strings.Contains(text, "do not raise it") || !strings.Contains(text, "paging") {
		t.Errorf("the intermittent source is not distinguished:\n%s", text)
	}
}

// An emitter with no path loss supplied is a wiring mistake. Defaulting it to
// zero would put a 57 dBm paging transmitter directly on the receiver.
func TestMissingPathLossIsAnError(t *testing.T) {
	_, err := rf.Interference(
		[]rf.Emitter{{Name: "paging", CentreHz: 869.525e6, BandwidthHz: 25e3, ERPdBm: 57}},
		map[string]float64{}, channel())
	if err == nil {
		t.Fatal("an emitter with no path loss was accepted")
	}
	if !strings.Contains(err.Error(), "paging") {
		t.Errorf("the error should name the emitter: %v", err)
	}
}
