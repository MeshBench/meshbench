package boardcheck

import (
	"context"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/mesh/packet"
	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// What does the board say it did with the advert it heard?
//
// The flood row passes about one run in three, and the two outcomes are not
// distinguishable from outside: the board adverts on its own timer either way,
// and the engine sees a transmission either way. The firmware's own console is
// the only account of whether it queued a relay and something ate it, or
// whether it decided not to.
func TestWhatTheBoardSaysAboutTheAdvert(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}
	board := os.Getenv("MESHCORESIM_BOARD")
	if board == "" {
		t.Skip("set MESHCORESIM_BOARD")
	}
	version := os.Getenv("MESHCORESIM_BOARD_VERSION")
	if version == "" {
		version = "v1.17.1"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
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
	under, ok := e.NodeByName("bc-under-test")
	if !ok || under.Firmware == nil {
		t.Fatal("the board never came up")
	}
	sender, ok := e.NodeByName("bc-sender")
	if !ok || sender.Firmware == nil {
		t.Fatal("the sender never came up")
	}
	said, ok := under.Firmware.Backend.(interface{ ConsoleLog() ([]byte, error) })
	if !ok {
		t.Fatal("this backend keeps no console, so it can say nothing")
	}

	// The flood row turns the sender down so the listener cannot hear it,
	// which also thins the board's own margin. Whether that is what makes the
	// row intermittent is the question, so it is a knob here rather than a
	// constant.
	if p := os.Getenv("MESHCORESIM_SENDER_TX"); p != "" {
		_ = sender.Firmware.Bridge.Type([]byte("set tx " + p + "\r\n"))
		settle(ctx, e, 1_000)
		t.Logf("the sender was turned to %s dBm", p)
	}

	settle(ctx, e, 90_000)

	// The row this reproduces runs an rx phase first, in which the sender
	// adverts once on its own clock. That advert is the only thing between
	// this test, which relays every time, and the row, which relays about one
	// run in four - so it is a knob rather than an assumption.
	if gap := os.Getenv("MESHCORESIM_RX_PHASE"); gap != "" {
		secs := 30
		if n, err := strconv.Atoi(gap); err == nil {
			secs = n
		}
		_ = sender.Firmware.Bridge.Type([]byte("advert\r\n"))
		settle(ctx, e, uint32(secs)*1000)
		t.Logf("the sender adverted once beforehand, %d s before this one", secs)
	}

	_ = sender.Firmware.Bridge.Type([]byte("time 1754703600\r\n"))
	settle(ctx, e, 1_000)
	mark := e.NowMs()
	if err := sender.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
		t.Fatalf("commanding the sender: %v", err)
	}

	chip := func(when string) {
		st := under.Firmware.Bridge.Stats()
		t.Logf("chip %-18s irqReads=%-7d busyReads=%-5d busyMs=%-6d mask=0x%04X flags=0x%04X  %s",
			when, st.IRQReads, st.BusyReads, st.BusyMs, st.IRQMask, st.IRQFlags, decode(st.IRQFlags))
	}
	chip("before the advert")

	fromSender := map[uint64]bool{}
	_, relayed := waitForEvent(ctx, e, advertBudgetMs, func(ev engine.Event) bool {
		if ev.Kind != "tx" {
			return false
		}
		if ev.From == "bc-sender" {
			fromSender[ev.MessageID] = true
			return false
		}
		return ev.From == "bc-under-test" && fromSender[ev.MessageID]
	})
	chip("after the window")
	t.Logf("relayed the sender's message: %v", relayed)

	for _, ev := range e.Events() {
		if ev.AtMs < mark {
			continue
		}
		if ev.From == "bc-under-test" || ev.To == "bc-under-test" {
			d := packet.Dissect(ev.Frame)
			t.Logf("engine %8.1fs %-4s %-14s -> %-14s id=%016x  type=%d route=%d path=%d payload=%d  %s",
				float64(ev.AtMs)/1000, ev.Kind, ev.From, ev.To, ev.MessageID,
				d.PayloadType, d.RouteType, len(d.PathHashes), len(d.Payload),
				hex.EncodeToString(ev.Frame))
		}
	}

	log, err := said.ConsoleLog()
	if err != nil {
		t.Fatalf("reading the console: %v", err)
	}
	t.Logf("---- the board's console, %d bytes ----", len(log))
	for _, line := range strings.Split(string(log), "\n") {
		if line = strings.TrimRight(line, "\r"); line != "" {
			t.Logf("| %s", line)
		}
	}
}
