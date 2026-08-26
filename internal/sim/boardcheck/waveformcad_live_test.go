package boardcheck

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// TestWaveformCADIsWhatTheFirmwareCarrierSenses is the firmware-in-the-loop half
// of MS4. TestWaveformCADTracksTheAir (engine) holds the busy vector to the air
// at the sample level; this watches real MeshCore firmware carrier-sense it.
//
// Two native repeaters in waveform mode, close enough to hear each other. In
// waveform mode "the air is busy" is decided by dechirped CAD on the actual
// summed IQ (dsp.CADBusy, waveformbusy.go), not by an engine rule - so the
// listener's own BusyReads (the driver's count of interrupt reads that found a
// detection flag set) should climb while the talker is on the air and stop when
// it goes quiet - decided by the actual channel energy, with no engine rule
// mediating.
//
// Live only: it runs native firmware. Set MESHBENCH_LIVE=1.
func TestWaveformCADIsWhatTheFirmwareCarrierSenses(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	ver := os.Getenv("MESHBENCH_BOARD_VERSION")
	if ver == "" {
		ver = nativePeerVersion
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	mast := antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical"}
	radio := scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62_500, SpreadFactor: 8, CodingRate: 4}
	node := func(name string, lon, txDBm float64) scenario.Node {
		return scenario.Node{
			Name: name, Kind: scenario.SimpleRepeater,
			Position: scenario.LatLon{Lat: 56.70, Lon: lon}, HeightAGLm: 10,
			Antenna: mast, TxPowerDBm: txDBm, NoiseFigureDB: 6, Radio: radio,
			Firmware: scenario.FirmwareRef{Role: "simple_repeater", Version: ver},
		}
	}
	// About 3 km apart at 20 dBm from a 6 dBi mast: the listener hears the
	// talker with margin, so a transmission plainly occupies its air.
	nodes := []scenario.Node{node("wf-talker", -3.90, 20), node("wf-listener", -3.85, 20)}

	e := engine.New(flat{}, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62_500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417, UnverifiedWiring: true,
		RFMode: engine.RFWaveform,
	})
	defer func() { _ = e.Close() }()
	for _, n := range nodes {
		e.Add(n, nil)
	}
	if err := e.AttachNative(ctx, 4433); err != nil {
		t.Fatalf("attaching native firmware: %v", err)
	}
	talker, ok := e.NodeByName("wf-talker")
	if !ok || talker.Firmware == nil {
		t.Fatal("the talker never came up")
	}
	listener, ok := e.NodeByName("wf-listener")
	if !ok || listener.Firmware == nil {
		t.Fatal("the listener never came up")
	}

	// busyReads returns the listener's carrier-sense count and logs the row.
	busyReads := func(label string) uint32 {
		st := listener.Firmware.Bridge.Stats()
		t.Logf("listener %-22s irqReads=%-7d busyReads=%-6d busyMs=%-6d",
			label, st.IRQReads, st.BusyReads, st.BusyMs)
		return st.BusyReads
	}

	// One clock, so both agree on airtime.
	_ = talker.Firmware.Bridge.Type([]byte("time 1754703600\r\n"))
	settle(ctx, e, 2_000)

	const bursts = 4
	burst := func(label string) uint32 {
		start := busyReads(label + " (before)")
		for i := 0; i < bursts; i++ {
			_ = talker.Firmware.Bridge.Type([]byte("advert\r\n"))
			settle(ctx, e, 3_000)
		}
		return busyReads(label+" (after)") - start
	}

	loudRise := burst("loud burst")

	// Quiet: the air clears, so busy stops climbing.
	beforeQuiet := busyReads("start of quiet")
	settle(ctx, e, 6_000)
	quietRise := busyReads("end of quiet") - beforeQuiet

	t.Logf("busyReads rose %d under a loud burst and %d over the quiet that followed",
		loudRise, quietRise)

	if loudRise == 0 {
		t.Errorf("the listener never found the air busy while the talker sent - CAD is not tracking the waveform")
	}
	if quietRise >= loudRise {
		t.Errorf("busy climbed as fast in the quiet (%d) as under traffic (%d)", quietRise, loudRise)
	}
}
