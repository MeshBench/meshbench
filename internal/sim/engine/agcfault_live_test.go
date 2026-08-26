package engine_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// The 1.17.1 receive-gain fault, provoked on purpose and watched at the register.
//
// The one precondition is agc_reset_interval, which defaults to 0 and guards
// the reset (Dispatcher.cpp:133). Nothing else has to be asked for, and that is
// the part worth stating plainly:
//
// simple_repeater sets rx_boosted_gain from the compile-time macro where there
// is one, and *to 1 otherwise* - MyMesh.cpp, "enabled by default" - then applies
// it through the wrapper, which is not guarded. So on a variant without the
// macro, boosted gain is on from boot with no operator involvement at all.
//
// The AGC reset then calibrates, which returns 0x08AC to its power-saving
// default, and re-applies nothing: SX126xReset.h wraps the restore in
// `#ifdef SX126X_RX_BOOSTED_GAIN`, so on exactly the variants that turned it on
// by default the restoring line is not compiled. Boosted gain is gone until
// reboot. 1.17.1 changed the argument that line passes and left it inside the
// same guard, which is why both versions behave identically below.
//
// Throughout, the firmware's own CLI goes on reporting the operator's value.
// _prefs.rx_boosted_gain is untouched; only the chip has changed. That is what
// makes it invisible without reading the register, and it is the whole reason
// kRadioStats carries one.
//
//	MESHBENCH_LIVE=1 go test ./internal/engine/ \
//	  -run TestAnAGCResetLosesBoostedGain -v -timeout 600s
func TestAnAGCResetLosesBoostedGain(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
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

	// Storage of its own, or this does not replicate anything.
	//
	// A node keeps its preferences between runs exactly as hardware does, so a
	// version sharing storage with one that ran before it boots holding that
	// run's rx_boosted_gain - a confound sitting on the precise register the
	// experiment is about. Fresh storage is also what makes the boot value
	// below evidence rather than an inheritance.
	fs := t.TempDir()
	old, had := os.LookupEnv("MESHBENCH_NODEFS")
	_ = os.Setenv("MESHBENCH_NODEFS", fs)
	defer func() {
		if had {
			_ = os.Setenv("MESHBENCH_NODEFS", old)
			return
		}
		_ = os.Unsetenv("MESHBENCH_NODEFS")
	}()

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
	// Boosted, on fresh storage, with nothing asked of it. This is the half of
	// the fault that is easy to miss: the chip's own reset default is 0x94, and
	// the repeater has moved it to 0x96 before anybody has typed anything.
	booted := gain()
	t.Logf("%6d ms  gain=%#02x  (fresh storage, nothing configured)", at, booted)
	if booted != 0x96 {
		t.Fatalf("booted holding %#02x, wanted 0x96: this variant defines no "+
			"SX126X_RX_BOOSTED_GAIN, so simple_repeater should have enabled "+
			"boosted gain by default", booted)
	}

	// Redundant here, and sent anyway: it is what an operator would do, and it
	// proves the register is reachable from the CLI as well as from the default.
	say("set radio.rxgain on")
	step(3000)
	before := gain()
	t.Logf("%6d ms  gain=%#02x  (after `set radio.rxgain on`)", at, before)
	if before != 0x96 {
		t.Fatalf("boosted gain is not on the chip: register is %#02x, wanted 0x96 - "+
			"nothing below this line would mean anything", before)
	}

	// The only precondition the fault has. 4 seconds: the CLI stores seconds/4
	// and the repeater multiplies by 4000 ms, so this is a reset every four.
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
