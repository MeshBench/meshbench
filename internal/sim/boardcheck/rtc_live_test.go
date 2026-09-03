package boardcheck

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/mesh/packet"
	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// The flood row moves the sender's clock so each advert it asks for is a packet
// nobody has seen - a new timestamp is a new payload hash. For that to hold the
// clock has to do two things, and this checks both by reading the timestamp back
// off the air from the adverts it triggers:
//
//   - take a time set to the second, so hours, minutes and seconds are honoured
//     (the first sample, checked against the value just set); and
//   - tick on its own afterwards, so a later advert carries a later time without
//     the clock being touched again (the remaining samples, checked against the
//     real wall-clock time that has passed).
//
// A clock frozen at whatever it booted with would pass a set-and-read-back check
// - the read happens moments after the set - but not this one: after a minute of
// real time its adverts would still carry the first instant, and the drift below
// would be the whole minute.
//
//	MESHBENCH_LIVE=1 go test ./internal/sim/boardcheck -run TestRTCTicks -v
func TestRTCTicks(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	tools := filepath.Join(os.Getenv("HOME"), ".cache", "meshbench", "tools")
	if os.Getenv("MESHBENCH_RADIO_SERVER") == "" {
		t.Setenv("MESHBENCH_RADIO_SERVER", filepath.Join(tools, "radioserver"))
	}

	e := engine.New(flatEarth{}, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62_500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417, UnverifiedWiring: true,
	})
	defer func() { _ = e.Close() }()
	// One native node is enough - only the sender's own clock is under test.
	for _, n := range probeGeometry("Ebyte_EoRa-S3", "v1.17.1") {
		e.Add(n, nil)
	}
	ctx := t.Context()
	if err := e.AttachNative(ctx, 4417); err != nil {
		t.Fatalf("attach: %v", err)
	}
	sender, ok := e.NodeByName("bc-sender")
	if !ok || sender.Firmware == nil {
		t.Fatal("sender never came up")
	}
	// Let it finish booting so its own boot advert is out of the way.
	_, _ = waitForEvent(ctx, e, advertBudgetMs, func(ev engine.Event) bool {
		return ev.Kind == "tx" && ev.From == "bc-sender"
	})

	// Set the clock once to a known instant, note the real time that set happened,
	// and never touch it again. A ticking clock keeps its advert timestamps close
	// to base + (real seconds since the set); a frozen one falls a whole run behind.
	const base int64 = 1754800000 // 2025-08-10 04:26:40 UTC
	wall0 := time.Now()
	if err := setSenderClock(sender, base); err != nil {
		t.Fatalf("set clock: %v", err)
	}
	_ = e.Run(ctx, e.NowMs()+1_000)

	var first, lastTs uint32
	for k := 0; k < 4; k++ {
		// waitUntilQuiet plus the advert round-trip spend real time between
		// samples - that elapsed time is exactly what the clock is expected to
		// have counted.
		_, _ = waitUntilQuiet(ctx, e, "bc-sender", floodQuietMs, 30_000)
		ts := readAdvertTs(ctx, t, e, sender)
		elapsed := int64(time.Since(wall0).Seconds())
		expect := base + elapsed
		// The advert is stamped ~1.5 s before this read (its transmission), so the
		// timestamp trails "now" by the advert delay; the window allows for that
		// and for the engine's stepping. What it does not allow is the clock not
		// having moved - that shows up as drift near -elapsed.
		drift := int64(ts) - expect
		tsT := time.Unix(int64(ts), 0).UTC()
		t.Logf("sample %d: %2ds after the set, advert stamped %02d:%02d:%02d (%d), drift %+ds",
			k, elapsed, tsT.Hour(), tsT.Minute(), tsT.Second(), ts, drift)
		if drift < -8 || drift > 8 {
			t.Errorf("RTC is not tracking real time: %d s after setting the clock to %d the "+
				"advert should carry about %d but carried %d (drift %+ds) - the clock did not tick",
				elapsed, base, expect, ts, drift)
		}
		if k == 0 {
			first = ts
		}
		lastTs = ts
	}
	// A frozen clock that happened to boot near base could sit inside the window
	// on every sample; that it advanced across the run rules that out.
	if lastTs <= first {
		t.Errorf("advert timestamp did not advance over the run: first %d, last %d - the clock is not ticking",
			first, lastTs)
	}
}

// readAdvertTs asks the sender to advert and returns the 4-byte epoch the advert
// actually carried on the air. It does not touch the clock.
func readAdvertTs(ctx context.Context, t *testing.T, e *engine.Engine, sender *engine.Node) uint32 {
	t.Helper()
	if err := sender.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
		t.Fatalf("advert: %v", err)
	}
	var stamp uint32
	_, out := waitForEvent(ctx, e, 60_000, func(ev engine.Event) bool {
		if ev.Kind != "tx" || ev.From != "bc-sender" || len(ev.Frame) == 0 {
			return false
		}
		d := packet.Dissect(ev.Frame)
		if d.PayloadType != 0x04 || len(d.Payload) < 36 { // 0x04 = advert
			return false
		}
		stamp = binary.LittleEndian.Uint32(d.Payload[32:36])
		return true
	})
	if out != eventMatched {
		t.Fatalf("no advert seen from the sender (outcome %v)", out)
	}
	return stamp
}
