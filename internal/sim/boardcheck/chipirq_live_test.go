package boardcheck

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// SX1262 interrupt bits, so the numbers below read as what the chip means.
const (
	irqTxDone   = 0x0001
	irqRxDone   = 0x0002
	irqPreamble = 0x0004
	irqSyncWord = 0x0008
	irqHeaderOK = 0x0010
	irqCRCErr   = 0x0040
	irqTimeout  = 0x0200
)

// Does the chip ever register a reception?
//
// No emulated board relays, on either emulator. The pin an nRF52 waits on never
// moves - measured - but the ESP32 firmware does not use the pin, so that
// cannot be the whole story. This asks the chip itself, through the counters
// radioserver already reports: what the firmware asked to be told about, and
// what actually happened.
func TestTheChipReportsWhatItSaw(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	board := os.Getenv("MESHBENCH_BOARD")
	if board == "" {
		t.Skip("set MESHBENCH_BOARD")
	}
	version := os.Getenv("MESHBENCH_BOARD_VERSION")
	if version == "" {
		version = "v1.17.1"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	e := engine.New(flat{}, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62_500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417, UnverifiedWiring: true,
	})
	defer func() { _ = e.Close() }()
	for _, n := range probeGeometry(board, version) {
		e.Add(n, nil)
	}
	if err := e.AttachNative(ctx, 4421); err != nil {
		t.Fatalf("attaching: %v", err)
	}
	under, ok := e.NodeByName("bc-under-test")
	if !ok || under.Firmware == nil {
		t.Fatal("the board never came up")
	}
	sender, ok := e.NodeByName("bc-sender")
	if !ok || sender.Firmware == nil {
		t.Fatal("the sender never came up")
	}

	// A board that is not actually running answers every counter with zero,
	// which reads exactly like a chip that saw nothing. Say which backend is
	// under the node before believing any of it.
	t.Logf("backend: %T", under.Firmware.Backend)
	if _, emulated := under.Firmware.Backend.(interface{ ConsolePath() string }); !emulated {
		t.Fatalf("this is not an emulated board - the zeros below would mean nothing")
	}

	show := func(when string) {
		s := under.Firmware.Bridge.Stats()
		// busyReads and busyMs are the firmware's own view of how often the air
		// looked occupied. A board that thinks the channel is never clear
		// behaves exactly like one that cannot hear: it does not transmit, and
		// on nRF52 it puts its radio to sleep between samples.
		t.Logf("%-22s irqReads=%-7d busyReads=%-7d busyMs=%-8d mask=0x%04X flags=0x%04X  %s",
			when, s.IRQReads, s.BusyReads, s.BusyMs, s.IRQMask, s.IRQFlags, decode(s.IRQFlags))
	}

	settle(ctx, e, 90_000)
	show("after boot")

	_ = sender.Firmware.Bridge.Type([]byte("time 1754703600\r\n"))
	settle(ctx, e, 1_000)
	if err := sender.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
		t.Fatalf("commanding the sender: %v", err)
	}
	heard := false
	if _, ok := waitForEvent(ctx, e, 60_000, func(ev engine.Event) bool {
		return ev.Kind == "rx" && ev.To == "bc-under-test"
	}); ok {
		heard = true
	}
	show("after an advert")
	t.Logf("the engine says the channel delivered it: %v", heard)

	settle(ctx, e, 120_000)
	show("two minutes later")
}

// settle burns a budget at the pace an emulator can keep up with. e.Run alone
// advances virtual time as fast as the host will go, which for a board under
// Renode means no real time to run in - and a board that has not run answers
// every counter with zero, which reads exactly like a chip that saw nothing.
func settle(ctx context.Context, e *engine.Engine, ms uint32) {
	waitForEvent(ctx, e, ms, func(engine.Event) bool { return false })
}

func decode(f uint16) string {
	out := ""
	for _, b := range []struct {
		bit  uint16
		name string
	}{
		{irqTxDone, "TxDone"}, {irqRxDone, "RxDone"}, {irqPreamble, "Preamble"},
		{irqSyncWord, "SyncWord"}, {irqHeaderOK, "HeaderOK"},
		{irqCRCErr, "CrcErr"}, {irqTimeout, "Timeout"},
	} {
		if f&b.bit != 0 {
			out += b.name + " "
		}
	}
	if out == "" {
		return "(nothing set)"
	}
	return out
}
