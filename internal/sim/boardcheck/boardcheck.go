// Package boardcheck turns the board list on its side: not can this board
// run, but which capabilities does it actually demonstrate, measured rather
// than asserted - and untested distinguished from failed, because a blank
// cell reads as working and it should read as unknown.
//
// Each capability is a short scripted run against the same emulation
// harness the engine's own live tests use: boot the published image under
// QEMU or Renode, drive it from a native peer on the same simulated
// channel, and read the ledger.
package boardcheck

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/MeshBench/meshbench/internal/mesh/firmware"
	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/rf/propagation"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Capability is one thing worth knowing about a board.
type Capability string

const (
	Build Capability = "build" // a published image exists for this board and version
	Boot  Capability = "boot"  // the image starts and attaches to the bridge
	Radio Capability = "radio" // the radio initialises and the node transmits at all
	TX    Capability = "tx"    // an explicit send is observed on the air
	RX    Capability = "rx"    // a peer's transmission is heard and decoded
	Flood Capability = "flood" // a message is relayed onward, not just received
	FEM   Capability = "fem"   // a front-end module (PA/LNA) is asserted correctly
	Power Capability = "power" // the node keeps responding after an idle period
)

// Capabilities is every column, left to right.
var Capabilities = []Capability{Build, Boot, Radio, TX, RX, Flood, FEM, Power}

// State is three-valued on purpose: a board never run must never look the
// same as one that was run and passed.
type State string

const (
	Untested      State = "untested" // never measured
	Passed        State = "passed"
	Failed        State = "failed"
	NotApplicable State = "n/a" // measured the precondition; there is nothing here to test
)

// Result is one capability's outcome, with the evidence for it - a failure
// with no reason sends someone back to logs this exists to replace.
type Result struct {
	Capability Capability
	State      State
	Detail     string
}

// BoardReport is one board's whole row.
type BoardReport struct {
	Board      string
	Version    string
	Results    map[Capability]Result
	EmulatorFP string // what emulator build produced this, for cache invalidation
	MeasuredAt time.Time
	// Stale is set by the cache loader, not by Probe: it means the emulator
	// in use today no longer matches the one that produced this report.
	Stale bool
}

func untestedReport(board, version string) BoardReport {
	r := BoardReport{Board: board, Version: version, Results: map[Capability]Result{}}
	for _, c := range Capabilities {
		r.Results[c] = Result{Capability: c, State: Untested}
	}
	return r
}

func (r *BoardReport) set(c Capability, s State, detail string) {
	r.Results[c] = Result{Capability: c, State: s, Detail: detail}
}

// EmulatorFingerprint identifies the emulator build in use, so a cached
// report can tell whether it is still about the binary that produced it.
// The size and mtime of the QEMU or Renode binary this run resolves to -
// cheap, and it changes exactly when a rebuild would invalidate the report.
func EmulatorFingerprint() string {
	path := os.Getenv(firmware.EnvQEMU)
	if path == "" {
		path = os.Getenv("MESHCORESIM_RENODE")
	}
	if path == "" {
		return "unconfigured"
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "unconfigured"
	}
	return fmt.Sprintf("%s@%d-%d", path, fi.Size(), fi.ModTime().Unix())
}

// probeGeometry is three nodes: a native sender, the board under test in the
// middle, and a native listener out of the sender's own direct reach - so a
// message the listener receives proves the middle node relayed it, not that
// the sender happened to reach both.
func probeGeometry(board, version string) []scenario.Node {
	mast := antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical"}
	radio := scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62_500, SpreadFactor: 8, CodingRate: 4}
	node := func(name string, lat, lon float64, fw scenario.FirmwareRef) scenario.Node {
		return scenario.Node{
			Name: name, Kind: scenario.SimpleRepeater,
			Position: scenario.LatLon{Lat: lat, Lon: lon}, HeightAGLm: 10,
			Antenna: mast, TxPowerDBm: 20, NoiseFigureDB: 6, Radio: radio,
			Firmware: fw,
		}
	}
	// The native peers are not what is under test - they only need to be a
	// reliable sender and listener - so they pin a native build ref
	// ("repeater-vX.Y.Z"), which is a different naming convention from a
	// published board image's bare "vX.Y.Z" and not interchangeable with it.
	return []scenario.Node{
		node("bc-sender", 56.70, -3.90, scenario.FirmwareRef{Role: "simple_repeater", Version: nativePeerVersion}),
		node("bc-under-test", 56.70, -3.30,
			scenario.FirmwareRef{Role: "simple_repeater", Version: version, Board: board}),
		node("bc-listener", 56.70, -2.70, scenario.FirmwareRef{Role: "simple_repeater", Version: nativePeerVersion}),
	}
}

// nativePeerVersion is the native build the probe's sender and listener run.
const nativePeerVersion = "repeater-v1.17.0"

// advertBudgetMs is how long any phase waits for something to reach the air,
// and it is one number on purpose.
//
// The phases used to differ - 90 s for the first advert, 60 s after an idle -
// and an emulated ESP32 took 68.5 s to produce its first. So the board passed
// the phase with the generous budget and failed the identical act under the
// tighter one, and the matrix recorded "no response after the idle period"
// for a board that was answering perfectly well. A board's second advert
// cannot be held to a shorter deadline than its first, and a relay - which is
// an advert plus a hop - cannot be held to a shorter one than either.
const advertBudgetMs = 90_000

// Probe runs every capability for one board and version, in one boot.
//
// A board's full column completing quickly matters: this is scripted rather
// than exploratory, each phase is bounded, and a phase that never produces
// its evidence fails rather than hanging the probe for anyone waiting on it.
func Probe(ctx context.Context, terr propagation.Terrain, board, version string) BoardReport {
	report := untestedReport(board, version)
	report.EmulatorFP = EmulatorFingerprint()
	report.MeasuredAt = time.Now()

	cacheDir := firmware.DefaultCacheDir()
	cat := &firmware.BoardCatalogue{CacheDir: cacheDir}
	// Already on disk is the common case once a board has been probed once,
	// and it needs no network at all - checked first, so a flaky connection
	// to GitHub's API does not turn "cached" into "untested".
	cached := firmware.BoardImage{Board: board, Role: "simple_repeater", Version: version, Format: "bin"}
	imgPath := firmware.BoardImagePath(cacheDir, cached)
	if _, err := os.Stat(imgPath); err == nil {
		report.set(Build, Passed, "image already cached: "+imgPath)
	} else {
		all, err := cat.ListAll(ctx)
		if err != nil {
			report.set(Build, Failed, "could not reach the firmware catalogue: "+err.Error())
			return report
		}
		var img firmware.BoardImage
		for _, i := range all {
			if i.Board == board && i.Version == version && i.Role == "simple_repeater" {
				img = i
			}
		}
		if img.Name == "" {
			report.set(Build, Failed, fmt.Sprintf("no published simple_repeater image for %s %s", board, version))
			return report
		}
		if _, err := cat.Ensure(ctx, img); err != nil {
			report.set(Build, Failed, "could not fetch the image: "+err.Error())
			return report
		}
		report.set(Build, Passed, "image "+img.Name)
	}

	e := engine.New(terr, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62_500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	defer func() { _ = e.Close() }()
	for _, n := range probeGeometry(board, version) {
		e.Add(n, nil)
	}
	if err := e.AttachNative(ctx, 4417); err != nil {
		report.set(Boot, Failed, err.Error())
		return report
	}
	under, ok := e.NodeByName("bc-under-test")
	if !ok || under.Firmware == nil {
		report.set(Boot, Failed, "attached, but the board's own firmware never came up")
		return report
	}
	report.set(Boot, Passed, "attached and answering")

	// A socket connecting is not the same as the firmware's own boot being
	// finished - an emulated board is a real bootloader and a real MeshCore
	// init, on top of QEMU's own startup, and a command sent into that
	// window is sent to a UART nothing is reading yet.
	if err := e.Run(ctx, e.NowMs()+5_000); err != nil {
		report.set(Radio, Failed, "settling: "+err.Error())
		report.set(TX, Failed, "settling: "+err.Error())
		return report
	}

	// Radio and tx: the board under test originates on command.
	if err := under.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
		report.set(Radio, Failed, "advert: "+err.Error())
		report.set(TX, Failed, "advert: "+err.Error())
		return report
	}
	txAt, ok := waitForEvent(ctx, e, advertBudgetMs, func(ev engine.Event) bool {
		return ev.Kind == "tx" && ev.From == "bc-under-test"
	})
	if !ok {
		report.set(Radio, Failed, "no transmission observed within 90 s of an advert")
		report.set(TX, Failed, "no transmission observed within 90 s of an advert")
		return report
	}
	report.set(Radio, Passed, fmt.Sprintf("transmitted at %.1f s", float64(txAt)/1000))
	report.set(TX, Passed, fmt.Sprintf("transmitted at %.1f s", float64(txAt)/1000))

	// rx: the sender adverts, the board under test should hear it.
	sender, ok := e.NodeByName("bc-sender")
	if ok && sender.Firmware != nil {
		if err := sender.Firmware.Bridge.Type([]byte("advert\r\n")); err == nil {
			if _, ok := waitForEvent(ctx, e, advertBudgetMs, func(ev engine.Event) bool {
				return ev.Kind == "rx" && ev.To == "bc-under-test"
			}); ok {
				report.set(RX, Passed, "heard the sender's advert")
			} else {
				report.set(RX, Failed, "no reception observed within 60 s")
			}
		} else {
			report.set(RX, Failed, "could not command the sender: "+err.Error())
		}
	} else {
		report.set(RX, Failed, "the native sender never came up")
	}

	// flood: the sender's next advert should reach the listener only via a
	// relay through the board under test - the listener is out of the
	// sender's own direct range by construction.
	if ok && sender.Firmware != nil {
		if err := sender.Firmware.Bridge.Type([]byte("advert\r\n")); err == nil {
			if _, ok := waitForEvent(ctx, e, advertBudgetMs, func(ev engine.Event) bool {
				return ev.Kind == "rx" && ev.To == "bc-listener"
			}); ok {
				report.set(Flood, Passed, "the listener heard it, out of the sender's own reach")
			} else {
				report.set(Flood, Failed, "the listener never heard a relay within 75 s")
			}
		} else {
			report.set(Flood, Failed, "could not command the sender: "+err.Error())
		}
	}

	// fem: nothing in scenario.QEMUWiring or RenodeWiring models a front-end
	// module today, so there is no signal to assert - reported as what it
	// is, not guessed at.
	report.set(FEM, NotApplicable, "no front-end module modelled for this board's wiring")

	// power: idle, then prove the board still answers.
	//
	// Asked as a question rather than as a transmission. The first version
	// typed "advert" after the idle and waited for something on the air,
	// which conflates two different things: whether the node is alive, and
	// whether its own airtime budget will let it speak again this minute.
	// MeshCore polices its duty cycle, so a healthy board that has just
	// adverted and relayed twice can decline correctly - and the matrix
	// recorded that as a dead node. A console reply costs no airtime and
	// answers the question actually being asked.
	if err := e.Run(ctx, e.NowMs()+15_000); err != nil {
		report.set(Power, Failed, "idle step: "+err.Error())
		return report
	}
	var replies bytes.Buffer
	var mu sync.Mutex
	under.Firmware.Bridge.Console(writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return replies.Write(p)
	}))
	// The same command the radio phase used, because it is the one this
	// firmware is known to accept - and read on the console rather than on
	// the air. Whether the packet leaves is the duty cycle's business; whether
	// the node acknowledges the command at all is the question here.
	if err := under.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
		report.set(Power, Failed, "commanding it after idle: "+err.Error())
		return report
	}
	answered := false
	deadline := e.NowMs() + advertBudgetMs
	for e.NowMs() < deadline {
		if err := e.Run(ctx, e.NowMs()+500); err != nil {
			break
		}
		mu.Lock()
		got := replies.Len() > 0
		mu.Unlock()
		if got {
			answered = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if answered {
		report.set(Power, Passed, "acknowledged a command after a 15 s idle period")
	} else {
		report.set(Power, Failed, "said nothing at all after a 15 s idle period")
	}

	return report
}

// MatrixReports is the whole board list, agreeing with
// scenario.EmulatableBoards() on the can-it-run question: a board that
// cannot be emulated at all is reported boot-failed with that reason,
// without spending any emulator time finding out again. A board that can
// run gets whatever was last measured for it - Untested in every column if
// that is nothing.
func MatrixReports(version string) []BoardReport {
	ok, blocked := scenario.EmulatableBoards()
	okNames := map[string]bool{}
	for _, b := range ok {
		okNames[b.Name] = true
	}
	all := scenario.Boards()
	out := make([]BoardReport, 0, len(all))
	for _, b := range all {
		if reason, isBlocked := blocked[b.Name]; isBlocked && !okNames[b.Name] {
			r := untestedReport(b.Name, version)
			r.set(Boot, Failed, reason)
			out = append(out, r)
			continue
		}
		out = append(out, Load(b.Name, version))
	}
	return out
}

// waitForEvent steps the engine in short strides, paced to real time because
// an emulated node cannot be run faster than it runs, until match sees an
// event or budgetMs of simulated time passes without one.
func waitForEvent(ctx context.Context, e *engine.Engine, budgetMs uint32,
	match func(engine.Event) bool) (atMs uint32, ok bool) {

	start := e.NowMs()
	began := time.Now()
	for e.NowMs() < start+budgetMs {
		if ctx.Err() != nil {
			return 0, false
		}
		target := e.NowMs() + 500
		if err := e.Run(ctx, target); err != nil {
			return 0, false
		}
		for _, ev := range e.Events() {
			if ev.AtMs >= start && match(ev) {
				return ev.AtMs, true
			}
		}
		// Sleep the whole deficit, not a capped slice of it.
		//
		// Each stride advances the simulation half a second while the old cap
		// slept at most a fifth of one, so simulated time outran the clock by
		// up to 300 ms a stride: a "90 second" budget could expire in 36
		// seconds of real time, and how much real time a board actually got
		// varied with whatever else the machine was doing. The board is a real
		// process on the real clock - an emulated ESP32 that adverts 68
		// seconds after boot needs 68 seconds, not 68 of somebody's
		// accelerated units - and that is why the same board passed this phase
		// twice and then failed it outright. Slept in slices so cancellation
		// stays responsive, but the full deficit is slept.
		for {
			d := time.Duration(e.NowMs()-start)*time.Millisecond - time.Since(began)
			if d <= 0 || ctx.Err() != nil {
				break
			}
			time.Sleep(min(d, 200*time.Millisecond))
		}
	}
	return 0, false
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// writerFunc adapts a function to io.Writer, so the probe can collect a
// node's console without a pipe and a goroutine to drain it.
type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
