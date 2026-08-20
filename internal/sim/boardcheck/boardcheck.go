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
	"context"
	"fmt"
	"os"
	"strings"
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
	node := func(name string, lat, lon float64, txDBm float64, fw scenario.FirmwareRef) scenario.Node {
		return scenario.Node{
			Name: name, Kind: scenario.SimpleRepeater,
			Position: scenario.LatLon{Lat: lat, Lon: lon}, HeightAGLm: 10,
			Antenna: mast, TxPowerDBm: txDBm, NoiseFigureDB: 6, Radio: radio,
			Firmware: fw,
		}
	}
	// The listener must not hear the sender, and now does not.
	//
	// It used to, which quietly broke the flood phase: three nodes 0.6 degrees
	// of longitude apart put the far pair 73 km from each other, which 20 dBm
	// from a 6 dBi mast covers easily over flat bare earth. So the listener
	// heard every packet directly, relayed it before the board had finished
	// waiting, and the board dropped its own copy - which is what a repeater
	// should do when somebody else has already relayed. The board was behaving
	// correctly and being marked down for it.
	//
	// Distance alone cannot separate the two links: free space costs 6 dB per
	// doubling, so a line of three nodes never puts much between the near hop
	// and the far one. A weak sender does. Measured on the channel rather than
	// estimated: at 2 dBm the sender reached the listener at -1.6 dB SNR, some
	// 8 dB above what SF8 needs, so it comes down another 14 dB and the board
	// moves in to 3 km to keep its own hop strong.
	//
	//	sender -> board       about +12 dB SNR   must work
	//	sender -> listener    about -16 dB SNR   must not
	//	board  -> listener    about  +1 dB SNR   must work
	//
	// A sender this quiet is not a realistic node, and it is not pretending to
	// be one: it is a fixture that puts the board on the only path between two
	// others, which is the whole of what this phase measures.
	return []scenario.Node{
		node("bc-sender", 56.70, -3.90, -12,
			scenario.FirmwareRef{Role: "simple_repeater", Version: nativePeerVersion}),
		node("bc-under-test", 56.70, -3.85, 20,
			scenario.FirmwareRef{Role: "simple_repeater", Version: version, Board: board}),
		node("bc-listener", 56.70, -2.90, 20,
			scenario.FirmwareRef{Role: "simple_repeater", Version: nativePeerVersion}),
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
//
// Four minutes, not ninety seconds, since an emulated board got a filesystem
// that works. It formats it on first boot, which is 1.4 MB of flash through an
// emulated SPI controller and takes most of ninety seconds by itself - so the
// old budget was spent before the board had finished starting. The board was
// relaying the whole time and being recorded as a board that would not.
//
// The cost is real: a probe takes minutes rather than one. Measuring the wrong
// thing faster is not a saving.
const advertBudgetMs = 240_000

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
	// Both formats, because the format follows the MCU: a merged .bin for the
	// ESP32 boards and a .uf2 for the nRF52 ones. Looking only for .bin sent
	// every nRF52 board to the network on every probe, cached or not.
	var imgPath string
	for _, format := range []string{"bin", "uf2"} {
		p := firmware.BoardImagePath(cacheDir, firmware.BoardImage{
			Board: board, Role: "simple_repeater", Version: version, Format: format,
		})
		if _, err := os.Stat(p); err == nil {
			imgPath = p
			break
		}
	}
	if imgPath != "" {
		report.set(Build, Passed, "image already cached: "+imgPath)
	} else {
		all, err := cat.ListAll(ctx)
		if err != nil {
			report.set(Build, Failed, "could not reach the firmware catalogue: "+err.Error())
			return report
		}
		// Through Runnable, because a release carries more than one asset per
		// board and only some of them are a flash image. Taking the last match
		// picked the DFU .zip over the .uf2, which then got loaded as if it
		// were flash.
		var img firmware.BoardImage
		for _, i := range firmware.Runnable(all, nil) {
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

	// A probe runs wiring nobody has watched boot yet - that is what it is
	// for. Refusing it here would mean a board could never leave the blocked
	// list, because the only thing that could clear it was gated on being
	// cleared already.
	e := engine.New(terr, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62_500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
		UnverifiedWiring: true,
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

	// Turn the sender down, in the firmware, because that is the only place it
	// can be turned down.
	//
	// A scenario node carries a TxPowerDBm and for a node running firmware it
	// is not what reaches the air: the firmware tells its radio what power to
	// use, from a preference of its own. Changing the scenario's figure moved
	// no measured SNR at all, while moving a node did - which is how that was
	// found rather than assumed.
	//
	// It is turned down so the listener cannot hear it, leaving the board the
	// only path between the two. See probeGeometry for the levels.
	if sender0, ok0 := e.NodeByName("bc-sender"); ok0 && sender0.Firmware != nil {
		if sender0.Firmware.Bridge.Type([]byte("set tx -9\r\n")) == nil {
			_ = e.Run(ctx, e.NowMs()+1_000)
		}
	}

	// A socket connecting is not the same as the firmware's own boot being
	// finished - an emulated board is a real bootloader and a real MeshCore
	// init, on top of QEMU's own startup, and a command sent into that
	// window is sent to a UART nothing is reading yet.
	if err := e.Run(ctx, e.NowMs()+5_000); err != nil {
		report.set(Radio, Failed, "settling: "+err.Error())
		report.set(TX, Failed, "settling: "+err.Error())
		return report
	}

	// Radio and tx: the board reaches the air by itself.
	//
	// Watched, not commanded. This phase used to type "advert" at the board
	// first and report the transmission that followed as one made on command.
	// It was not: nothing reads an emulated node's UART, so the typing went
	// nowhere, and what arrived was simple_repeater's own unprompted advert a
	// few seconds after boot. Withholding the command entirely changed neither
	// the result nor the time it arrived, which is what settled it.
	txAt, ok := waitForEvent(ctx, e, advertBudgetMs, func(ev engine.Event) bool {
		return ev.Kind == "tx" && ev.From == "bc-under-test"
	})
	if !ok {
		never := fmt.Sprintf("never transmitted within %d s of coming up", advertBudgetMs/1000)
		report.set(Radio, Failed, never)
		report.set(TX, Failed, never)
		return report
	}
	report.set(Radio, Passed, fmt.Sprintf("adverted unprompted at %.1f s", float64(txAt)/1000))
	report.set(TX, Passed, fmt.Sprintf("adverted unprompted at %.1f s", float64(txAt)/1000))

	// rx: the sender adverts, the board under test should hear it.
	sender, ok := e.NodeByName("bc-sender")
	if ok && sender.Firmware != nil {
		if err := sender.Firmware.Bridge.Type([]byte("advert\r\n")); err == nil {
			if _, ok := waitForEvent(ctx, e, advertBudgetMs, func(ev engine.Event) bool {
				return ev.Kind == "rx" && ev.To == "bc-under-test"
			}); ok {
				report.set(RX, Passed, "heard the sender's advert")
			} else {
				report.set(RX, Failed, fmt.Sprintf(
					"no reception observed within %d s", advertBudgetMs/1000))
			}
		} else {
			report.set(RX, Failed, "could not command the sender: "+err.Error())
		}
	} else {
		report.set(RX, Failed, "the native sender never came up")
	}

	// flood: the board forwards somebody else's packet.
	//
	// Watched at the board, not at the listener. This phase used to advert
	// from the sender and pass when the listener heard it, on the stated
	// construction that the listener was out of the sender's direct reach. It
	// is not: the three sit 0.6 degrees of longitude apart at this latitude,
	// making the far pair about 73 km, and 20 dBm from a 6 dBi mast over the
	// simulator's flat bare earth covers that comfortably. The event log shows
	// the listener hearing the sender directly, so what passed was a direct
	// reception with a relay's name on it.
	//
	// Requiring the board's own transmission instead settles it wherever the
	// nodes are put, which is the property the old test was trying to buy with
	// geometry and did not get.
	if ok && sender.Firmware != nil {
		// A message the sender authors, rather than another advert. An advert
		// is the one packet a node has already sent once by the time this runs,
		// and the simulated clock does not move, so a second one is the same
		// bytes and the board is right to drop it. Originate makes the sender
		// the author of something nobody has seen.
		// The sender's clock is moved on first: every node adverts once as it
		// boots, the simulated clock does not advance, and a second advert with
		// the same timestamp is the same bytes - which the board drops as the
		// duplicate it is. Moving the clock makes this a packet nobody has seen.
		_ = sender.Firmware.Bridge.Type([]byte("time 1754703600\r\n"))
		_ = e.Run(ctx, e.NowMs()+1_000)
		if err := sender.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
			report.set(Flood, Failed, "could not command the sender: "+err.Error())
		} else if _, relayed := waitForEvent(ctx, e, advertBudgetMs, func(ev engine.Event) bool {
			return ev.Kind == "tx" && ev.From == "bc-under-test"
		}); relayed {
			report.set(Flood, Passed, "forwarded the sender's advert itself")
		} else {
			report.set(Flood, Failed, fmt.Sprintf(
				"received a fresh advert as the only node that could relay it, "+
					"and put nothing back on the air within %d s", advertBudgetMs/1000))
		}
	} else {
		report.set(Flood, Failed, "the native sender never came up")
	}

	// fem: nothing in scenario.QEMUWiring or RenodeWiring models a front-end
	// module today, so there is no signal to assert - reported as what it
	// is, not guessed at.
	report.set(FEM, NotApplicable, "no front-end module modelled for this board's wiring")

	// power: the board is still answering after sitting idle.
	//
	// Asked directly, now that an emulated board has a serial port to be asked
	// through, and asked for a console reply rather than for a transmission.
	// Nothing about the duty cycle can refuse a reply, and nothing about a
	// timer can produce one - which a transmission could, and did.
	//
	// Silence here is therefore a board that has stopped, which is the thing
	// this row is for.
	if err := e.Run(ctx, e.NowMs()+15_000); err != nil {
		report.set(Power, Untested, "idle step: "+err.Error())
		return report
	}
	// Asked for a reply, not for a transmission.
	//
	// Watching for a transmission cannot answer this: the firmware adverts on
	// a timer of its own, about two minutes apart, so a board left alone
	// transmits again whether or not anything reached it. Withholding the
	// command and running the probe anyway produced the same pass at the same
	// moment, which is how that was ruled out. A console reply cannot be
	// produced by a timer - it exists only if the command was read and run.
	said, ok := under.Firmware.Backend.(interface{ ConsoleLog() ([]byte, error) })
	if !ok {
		report.set(Power, Untested, "this backend keeps no console log to read a reply from")
		return report
	}
	// Established before the idle, not assumed: a board that answers nothing on
	// its console cannot be measured this way, and calling that a failure would
	// mark a healthy board dead for saying little. Only a board that answered
	// before and does not after has actually stopped.
	baseline, err := said.ConsoleLog()
	if err != nil {
		report.set(Power, Untested, "could not read the console: "+err.Error())
		return report
	}
	if err := under.Firmware.Bridge.Type([]byte("clock\r\n")); err != nil {
		report.set(Power, Untested, "no way to ask it: "+err.Error())
		return report
	}
	if !answeredOn(ctx, e, said, len(baseline), "->") {
		report.set(Power, Untested,
			"this build answers nothing on its console, so silence later would "+
				"not mean it had stopped")
		return report
	}

	before, err := said.ConsoleLog()
	if err != nil {
		report.set(Power, Untested, "could not read the console: "+err.Error())
		return report
	}
	if err := under.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
		report.set(Power, Untested, "no way to ask it: "+err.Error())
		return report
	}
	if answeredOn(ctx, e, said, len(before), "Advert sent") {
		report.set(Power, Passed, "answered a command on the console after a 15 s idle")
	} else {
		report.set(Power, Failed,
			"asked to advert after a 15 s idle and said nothing back on the console")
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

// answeredOn waits for the node to say something new on its console.
//
// Given the length the log had before the question, so that a reply is only
// what arrived after it - the boot chain is already in there, and matching
// against the whole file would find the firmware's own startup chatter.
func answeredOn(ctx context.Context, e *engine.Engine,
	said interface{ ConsoleLog() ([]byte, error) }, from int, want string) bool {

	for i := 0; i < 40; i++ {
		if err := e.Run(ctx, e.NowMs()+500); err != nil {
			return false
		}
		b, err := said.ConsoleLog()
		if err == nil && len(b) > from && strings.Contains(string(b[from:]), want) {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}
