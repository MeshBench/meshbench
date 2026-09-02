// The waiting a phase is made of, checked against a node without an emulator.
//
// Every capability below build is a wait: run the engine on, watch the ledger,
// and decide from what did or did not arrive. That decision is ordinary Go and
// it was reachable only through a probe of a real board under QEMU, which is
// why none of it ran anywhere. A stand-in for the native child process reaches
// all of it - the boards are what need an emulator, and no board is involved in
// deciding what a silence means.
package boardcheck

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/firmware/fakenative"
	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// TestMain lets this binary be re-entered as the node under it. The
// environment is read before the testing package parses flags, because a node
// is launched with MeshCore's arguments and a test binary handed --bridge
// would refuse to start at all.
func TestMain(m *testing.M) {
	if fakenative.Mode() != "" {
		os.Exit(fakenative.Serve())
	}
	os.Exit(m.Run())
}

// standInMesh is one node running the stand-in child, on a real engine.
//
// Real in every respect a phase depends on: the engine's clock, its ledger and
// its event log are the ones a probe reads, and the node answers the bridge in
// lockstep as a native peer does. Only what the node itself decides is
// missing, and no phase asks it anything.
func standInMesh(ctx context.Context, t *testing.T, txAtMs uint32) *engine.Engine {
	t.Helper()
	t.Setenv(fakenative.EnvMode, fakenative.ModeAdvert)
	t.Setenv(fakenative.EnvTxAtMs, fmt.Sprint(txAtMs))
	// Resolved rather than downloaded: an explicit binary is what the
	// environment override is for, and it is what keeps this off the network.
	t.Setenv(firmware.EnvNativeBinary, fakenative.Path())
	// Somewhere disposable for the node's own storage, so a test cannot inherit
	// or leave behind an identity.
	t.Setenv(firmware.EnvNodeFS, nodeFSRoot(t))

	e := engine.New(flat{}, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62_500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	// No stagger. It exists so that real nodes do not all start their advert
	// timers on the same millisecond, and here it would only make the moment
	// the stand-in transmits depend on a hash of the seed.
	e.StaggerBoot = false
	e.Add(scenario.Node{
		Name: "bc-sender", Kind: scenario.SimpleRepeater,
		Position: scenario.LatLon{Lat: 56.70, Lon: -3.90}, HeightAGLm: 10,
		Antenna:    antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical"},
		TxPowerDBm: 20, NoiseFigureDB: 6,
		Radio:    scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62_500, SpreadFactor: 8, CodingRate: 4},
		Firmware: scenario.FirmwareRef{Role: "simple_repeater", Version: nativePeerVersion},
	}, nil)
	t.Cleanup(func() { _ = e.Close() })

	if err := e.AttachNative(ctx, 4417); err != nil {
		t.Fatalf("attach the stand-in node: %v", err)
	}
	return e
}

// The positive half, so that the negatives elsewhere mean something: a phase
// that watches for a transmission does see one when it happens.
func TestWaitForEventSeesTheNodesOwnTransmission(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	e := standInMesh(ctx, t, 1_000)

	atMs, outcome := waitForEvent(ctx, e, 10_000, func(ev engine.Event) bool {
		return ev.Kind == "tx" && ev.From == "bc-sender"
	})
	if outcome != eventMatched {
		t.Fatalf("the node transmitted and the wait reported %v", outcome)
	}
	if atMs < 1_000 {
		t.Errorf("the transmission is recorded at %d ms, before it was made", atMs)
	}
}

// The flood phase's precondition, which is why three runs in four used to fail
// on a board that was behaving.
//
// The channel is half duplex, so a packet handed to a node that is on the air
// itself is a miss in the ledger and nothing the node ever heard. waitUntilQuiet
// is what stops the phase judging a board on a stimulus it was never given, and
// it has to wait out a transmission that is already under way rather than
// reporting the quiet it starts in.
func TestWaitUntilQuietWaitsOutATransmissionAlreadyUnderWay(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	e := standInMesh(ctx, t, 1_000)

	quiet, cancelled := waitUntilQuiet(ctx, e, "bc-sender", 1_000, 20_000)
	if cancelled {
		t.Fatal("the wait was cut short, so it says nothing about the node")
	}
	if !quiet {
		t.Fatal("a node that transmits once was never reported as having gone quiet")
	}
	var last uint32
	for _, ev := range e.Events() {
		if ev.Kind == "tx" && ev.From == "bc-sender" && ev.AtMs > last {
			last = ev.AtMs
		}
	}
	if last == 0 {
		t.Fatal("the node never transmitted, so the wait proved nothing")
	}
	if e.NowMs() < last+1_000 {
		t.Errorf("reported quiet at %d ms, only %d ms after the last transmission",
			e.NowMs(), e.NowMs()-last)
	}
}

// A cancelled probe and a board that said nothing must not come back identical.
//
// They used to. Both returned (0, false), and Probe turned that into "never
// transmitted within 240 s of coming up" - a sentence about the hardware,
// written from evidence that only said the wait ended. What ended it most often
// was the caller's own deadline, so a probe cut off mid-phase filed a specific,
// confident, wrong finding against a board that was never given time to answer.
//
// Against a node whose transmission is far beyond either wait, so the only
// difference between the two is which of them ran out.
func TestWaitForEventTellsACancelledProbeFromASilentBoard(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// Late enough that neither wait below can reach it.
	e := standInMesh(ctx, t, 600_000)
	silent := func(ev engine.Event) bool { return ev.Kind == "tx" && ev.From == "bc-under-test" }

	_, expired := waitForEvent(ctx, e, 1_000, silent)

	cut, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	_, cancelled := waitForEvent(cut, e, 1_000, silent)

	if expired != eventTimedOut {
		t.Errorf("a budget that ran out reported %v, want eventTimedOut", expired)
	}
	if cancelled != eventCancelled {
		t.Errorf("a cancelled wait reported %v, want eventCancelled", cancelled)
	}
	if expired == cancelled {
		t.Fatal("the two still come back the same, so a phase cannot tell a " +
			"silent board from a probe that was cut off")
	}
}

// nodeFSRoot is where the nodes under test keep their files.
//
// Not t.TempDir(), which fails the test if its own cleanup cannot unlink -
// and on Windows it cannot, while a killed child is still letting go of its
// stderr.log. Unix removes an open file and Windows refuses to. What these
// tests are about is what the waits see, which they assert directly; a file
// that takes another moment to close is a different question and was failing
// them on that alone.
func nodeFSRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Briefly: the handle closes as the process finishes dying.
		for range 20 {
			if err := os.RemoveAll(dir); err == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Logf("could not remove %s; something is still holding a file in it", dir)
	})
	return dir
}

// The rx phase's precondition, and the reason it was failing a board that was
// behaving correctly.
//
// A tx event is recorded at the moment a transmission starts: AtMs is the
// start and the packet is on the air until AtMs plus its airtime. So matching
// a tx event does not mean the node has finished transmitting, and a phase
// that stimulates the board the instant it matches one is handing a packet to
// a radio that cannot hear it. The channel is half duplex, which is right -
// every real radio is - so the ledger records a miss, and a row whose single
// stimulus was missed then waits out its whole budget for an event that can
// never arrive.
//
// The flood row waits. The rx row did not, which is why rx failed on every run
// of a board whose radio was working.
func TestMatchingATxEventDoesNotMeanTheNodeIsOffTheAir(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	e := standInMesh(ctx, t, 1_000)

	atMs, outcome := waitForEvent(ctx, e, 30_000, func(ev engine.Event) bool {
		return ev.Kind == "tx" && ev.From == "bc-sender"
	})
	if outcome != eventMatched {
		t.Fatalf("the stand-in never transmitted: outcome %v", outcome)
	}

	// The event names its own airtime, which is the whole point: the node is
	// busy from atMs until atMs plus that, and the wait returned at the start.
	var airtimeMs float64
	for _, ev := range e.Events() {
		if ev.Kind == "tx" && ev.From == "bc-sender" && ev.AtMs == atMs {
			// "10 bytes, 279 ms on air". Go has no assignment suppression in
			// Sscanf, so the byte count is read and discarded.
			var bytes int
			if _, err := fmt.Sscanf(ev.Detail, "%d bytes, %f ms on air", &bytes, &airtimeMs); err != nil {
				t.Fatalf("a tx event does not say how long it is on air: %q", ev.Detail)
			}
		}
	}
	if airtimeMs <= 0 {
		t.Fatal("the transmission reports no airtime, so this proves nothing")
	}
	if e.NowMs() >= atMs+uint32(airtimeMs) {
		t.Skip("the engine ran past the transmission before the wait returned; " +
			"the race this guards is real but not reproducible at this step size")
	}
	t.Logf("tx matched at %d ms with %.0f ms still to run: a stimulus sent now is missed",
		atMs, airtimeMs)

	// And this is what the phases must do instead.
	quiet, cancelled := waitUntilQuiet(ctx, e, "bc-sender", 1_000, 20_000)
	if cancelled || !quiet {
		t.Fatalf("waiting for quiet did not settle: quiet=%v cancelled=%v", quiet, cancelled)
	}
	if e.NowMs() < atMs+uint32(airtimeMs) {
		t.Errorf("reported quiet at %d ms, before the transmission that started at "+
			"%d ms had finished %.0f ms later", e.NowMs(), atMs, airtimeMs)
	}
}
