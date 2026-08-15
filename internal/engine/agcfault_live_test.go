package engine_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/antenna"
	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// The 1.17.1 receive-gain fault, provoked on purpose and watched at the register.
//
// It took an eight-cell sweep measuring nothing to work out why this never
// fired. Two preconditions, and only one of them was ever met:
//
//   - agc_reset_interval defaults to 0 and the reset is guarded on it
//     (Dispatcher.cpp:133), so AGC resets are off unless asked for. We did ask.
//   - rx_boosted_gain *also* defaults to 0 (CommonCLI.h:66). The fault destroys
//     a runtime setting, and we never made one - so the reset fired every four
//     seconds against a register already at its reset value, and the arms were
//     identical for a reason that had nothing to do with the firmware.
//
// With both set, the mechanism is: MeshCore reimplements RadioLib's AGC reset in
// SX126xReset.h and re-applies the *compile-time* SX126X_RX_BOOSTED_GAIN macro
// over whatever the operator set. The host variant does not define that macro,
// so the re-application is not merely wrong - it is `#ifdef`-ed out entirely and
// boosted gain is gone until reboot. That is the worse of the fault's two modes,
// and the native build is the right vehicle for it.
//
// Throughout, the firmware's own CLI goes on reporting the operator's value.
// _prefs.rx_boosted_gain is untouched; only the chip has changed. That is what
// makes it invisible without reading the register, and it is the whole reason
// kRadioStats carries one.
//
//	MESHCORESIM_LIVE=1 go test ./internal/engine/ \
//	  -run TestAnAGCResetLosesBoostedGain -v -timeout 600s
func TestAnAGCResetLosesBoostedGain(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}
	for _, version := range []string{"repeater-v1.17.0", "repeater-v1.17.1"} {
		t.Run(version, func(t *testing.T) {
			held := runAGCFault(t, version)
			t.Logf("%s: boosted gain %s the AGC reset", version, map[bool]string{
				true: "SURVIVED", false: "was LOST to"}[held])
		})
	}
}

// runAGCFault boots one repeater, turns boosted gain on, turns AGC resets on,
// and reports whether the register still holds boosted gain at the end.
func runAGCFault(t *testing.T, version string) bool {
	t.Helper()

	e := engine.New(flat{100}, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62_500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	defer func() { _ = e.Close() }()

	e.Add(scenario.Node{
		Name: "agc", Kind: scenario.SimpleRepeater,
		Position:   scenario.LatLon{Lat: 56.70, Lon: -3.90},
		HeightAGLm: 10,
		Antenna: antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6},
			Polarisation: "vertical"},
		TxPowerDBm: 22, NoiseFigureDB: 6,
		Radio: scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62_500,
			SpreadFactor: 8, CodingRate: 4},
		Firmware: scenario.FirmwareRef{Role: "simple_repeater", Version: version},
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	if err := e.AttachNative(ctx, 4417); err != nil {
		t.Skipf("no native %s to run: %v", version, err)
	}
	node := e.Nodes()[0]

	at := uint32(0)
	step := func(ms uint32) {
		for target := at + ms; at < target; at += 250 {
			if err := e.Run(ctx, at+250); err != nil {
				t.Fatalf("run to %d ms: %v", at+250, err)
			}
		}
	}
	say := func(cmd string) {
		if err := node.Firmware.Bridge.Type([]byte(cmd + "\r\n")); err != nil {
			t.Fatalf("%q: %v", cmd, err)
		}
	}
	gain := func() uint8 { return node.Firmware.Bridge.Stats().RxGainReg }

	// Let it boot and configure its modem before anything is asked of it.
	step(6000)
	if !node.Firmware.Bridge.Stats().Configured {
		t.Skip("this build does not report radio state; install a current release")
	}
	t.Logf("%6d ms  gain=%#02x  (after boot)", at, gain())

	// Precondition one. Without this the fault has nothing to destroy, which is
	// exactly how the first sweep came to measure nothing.
	say("set radio.rxgain on")
	step(3000)
	before := gain()
	t.Logf("%6d ms  gain=%#02x  (after `set radio.rxgain on`)", at, before)
	if before != 0x96 {
		t.Fatalf("boosted gain never reached the chip: register is %#02x, wanted 0x96 - "+
			"nothing below this line would mean anything", before)
	}

	// Precondition two. 4 seconds: the CLI stores seconds/4 and the repeater
	// multiplies by 4000 ms, so this is a reset every four seconds.
	say("set agc.reset.interval 4")

	// Long enough for several resets, sampled so the moment of the drop is
	// visible rather than only its aftermath.
	var dropped uint32
	last := before
	for i := 0; i < 12; i++ {
		step(2000)
		if g := gain(); g != last {
			t.Logf("%6d ms  gain=%#02x -> %#02x", at, last, g)
			if last == 0x96 && g != 0x96 && dropped == 0 {
				dropped = at
			}
			last = g
		}
	}
	t.Logf("%6d ms  gain=%#02x  (end)", at, last)
	if dropped != 0 {
		t.Logf("boosted gain was lost %d ms after the resets were enabled", dropped)
	}
	return last == 0x96
}
