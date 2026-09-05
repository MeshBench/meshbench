package boardcheck

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// Does the rate the firmware set its console to reach us?
//
// The firmware writes a clock divider; the machine turns that back into a
// number and sends it up with the rest of what the chip has been asked. This
// watches the whole of that: a board boots, MeshCore calls Serial.begin, and
// the figure arrives here.
//
// MeshCore asks for 115200, which is also the machine's own default - so the
// number alone proves less than it looks. What it proves is the field arrives
// at all: before the emulator carried it the stats record was four bytes
// shorter and this read zero.
func TestTheConsoleBaudReachesUs(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	e := engine.New(flatEarth{}, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62_500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417, UnverifiedWiring: true,
	})
	defer func() { _ = e.Close() }()
	for _, n := range probeGeometry(board, version) {
		e.Add(n, nil)
	}
	if err := e.AttachNative(ctx, 4436); err != nil {
		t.Fatalf("attaching: %v", err)
	}
	under, ok := e.NodeByName("bc-under-test")
	if !ok || under.Firmware == nil {
		t.Fatal("the board never came up")
	}

	// Every distinct rate, in order. The ROM bootloader, the second stage and
	// the application need not agree, so the first non-zero value is whichever
	// moment the reading happened to land in - and reading only that was how
	// two runs of this reported figures four times apart.
	// Read once the firmware has settled, and never before. The ROM
	// bootloader and the application need not agree about the rate, so an
	// early reading catches a divider nothing is using any more - this was
	// read at 230400 once, from one the bootloader had left behind.
	var got firmware.RadioStats
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		settle(ctx, e, 2_000)
		got = under.Firmware.Bridge.Stats()
		if got.Configured && got.ConsoleBaud != 0 {
			break
		}
	}
	t.Logf("console baud %d, radio configured=%v", got.ConsoleBaud, got.Configured)
	if got.ConsoleBaud == 0 {
		t.Fatal("no console rate arrived: either the emulator predates the " +
			"field, or nothing is wiring the console UART to the device that " +
			"reports it")
	}
	// MeshCore asks for 115200. The figure comes back through an integer
	// division so it lands beside rather than on it, and what is being checked
	// is that the clock it was computed against is the right one: the wrong
	// clock is wrong by a whole factor, not by one part in a hundred thousand.
	const want = 115200
	if got.ConsoleBaud < want*99/100 || got.ConsoleBaud > want*101/100 {
		t.Errorf("the console reports %d where the firmware asked for %d - a "+
			"figure that far out is a rate computed against the wrong clock",
			got.ConsoleBaud, want)
	}
}
