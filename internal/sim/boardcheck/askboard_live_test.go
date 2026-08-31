package boardcheck

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// Ask a board with a console what it did with the advert it was sent.
//
// The ESP32 board's chip demonstrably receives the packet - RxDone, SyncWord,
// HeaderOK, no CRC error - and the board forwards nothing. Everything after
// that point is MeshCore's own decision, and MeshCore keeps counters for
// exactly this: how many flood packets it received, how many it dropped as
// duplicates, how many it sent.
func TestAskTheBoardWhatItDidWithIt(t *testing.T) {
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
	if err := e.AttachNative(ctx, 4423); err != nil {
		t.Fatalf("attaching: %v", err)
	}
	under, _ := e.NodeByName("bc-under-test")
	sender, _ := e.NodeByName("bc-sender")
	if under == nil || under.Firmware == nil || sender == nil || sender.Firmware == nil {
		t.Fatal("the nodes did not come up")
	}
	said, ok := under.Firmware.Backend.(interface{ ConsoleLog() ([]byte, error) })
	if !ok {
		t.Skip("this backend keeps no console to ask through")
	}

	ask := func(cmd string) string {
		before, _ := said.ConsoleLog()
		if err := under.Firmware.Bridge.Type([]byte(cmd + "\r\n")); err != nil {
			t.Fatalf("no console: %v", err)
		}
		settle(ctx, e, 6_000)
		after, _ := said.ConsoleLog()
		return strings.TrimSpace(string(after[len(before):]))
	}

	settle(ctx, e, 90_000)
	t.Logf("stats-packets, before\n%s", ask("stats-packets"))

	// The probe adverts twice - once to prove reception, once to ask for a
	// relay - and only the second is timestamped. Reproduce that here, because
	// a board that relays a lone advert and not the second one is a different
	// fault from a board that relays nothing.
	// Stamped, as the probe's rx row now stamps it, and with a different time
	// from the second so the two are genuinely different packets.
	_ = sender.Firmware.Bridge.Type([]byte("time 1754700000\r\n"))
	settle(ctx, e, 1_000)
	if err := sender.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
		t.Fatalf("first advert: %v", err)
	}
	if _, outcome := waitForEvent(ctx, e, 60_000, func(ev engine.Event) bool {
		return ev.Kind == "rx" && ev.To == "bc-under-test"
	}); outcome != eventMatched {
		t.Fatal("the board never heard the first advert")
	}
	settle(ctx, e, 30_000)
	t.Logf("after the FIRST advert\n%s", ask("stats-packets"))

	_ = sender.Firmware.Bridge.Type([]byte("time 1754703600\r\n"))
	settle(ctx, e, 1_000)
	if err := sender.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
		t.Fatalf("commanding the sender: %v", err)
	}
	if _, outcome := waitForEvent(ctx, e, 60_000, func(ev engine.Event) bool {
		return ev.Kind == "rx" && ev.To == "bc-under-test"
	}); outcome != eventMatched {
		t.Fatal("the engine never delivered the advert")
	}
	settle(ctx, e, 30_000)

	// What the engine watched the board do, beside what the board says it did.
	// Without this the counters cannot be read: "sent 0" from a board that has
	// been heard on the air means the counter is not wired, and "sent 0" from a
	// board that has said nothing means something else entirely.
	var ownTx, ownRx int
	for _, ev := range e.Events() {
		if ev.From == "bc-under-test" && ev.Kind == "tx" {
			ownTx++
		}
		if ev.To == "bc-under-test" && ev.Kind == "rx" {
			ownRx++
		}
	}
	t.Logf("the engine watched the board transmit %d times and receive %d", ownTx, ownRx)

	t.Logf("after the SECOND advert\n%s", ask("stats-packets"))
	t.Logf("stats-radio\n%s", ask("stats-radio"))
	t.Logf("neighbors\n%s", ask("neighbors"))
}
