package boardcheck

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// flatEarth is the terrain a probe runs over: sea level everywhere, and
// answering everywhere (ok is always true, which is what separates it from the
// engine's bareEarth that answers "no data").
//
// A probe measures a board, not the ground under three imaginary nodes. Its
// geometry is calibrated on flat bare earth - the sender is turned down until
// the listener cannot hear it directly, leaving the board the only path - and
// those margins hold only over flat earth. Handed the operator's real cached
// tiles instead, the same weak sender sat behind whatever relief happened to be
// under the fixture's coordinates: in the Cairngorms, where the probe's nodes
// are, 370 m of hill added some 48 dB to the 3 km sender-to-board hop and drove
// a +29 dB margin to -18 dB, so the board could not hear the sender and every
// board failed rx and flood identically - not for anything the board did, but
// for the ground under a fixture, differently on every machine depending on
// which tiles it had cached. The probe owns its terrain so it cannot happen
// again, and so a board's result is the board's, reproducible anywhere.
type flatEarth struct{}

func (flatEarth) ElevationM(_, _ float64) (float64, bool) { return 0, true }

// Probe runs every capability for one board and version, in one boot.
//
// The columns are a sequence rather than a set: a board that will not boot has
// no radio to measure, and a board whose radio does not reach the air cannot
// be asked whether it forwards. So this reads top to bottom and stops where
// the evidence stops, which is why a failure names the column it happened in.
//
// Each phase is bounded and a phase that never produces its evidence fails,
// rather than hanging the probe for whoever is waiting on the column.
func Probe(ctx context.Context, board, version string) (report BoardReport) {
	report = untestedReport(board, version)
	report.EmulatorFP = EmulatorFingerprint()
	report.MeasuredAt = time.Now()

	cacheDir := firmware.DefaultCacheDir()
	cat := &emulated.BoardCatalogue{CacheDir: cacheDir}
	// Already on disk is the common case once a board has been probed once,
	// and it needs no network at all - checked first, so a flaky connection
	// to GitHub's API does not turn "cached" into "untested".
	// Both formats, because the format follows the MCU: a merged .bin for the
	// ESP32 boards and a .uf2 for the nRF52 ones. Looking only for .bin sent
	// every nRF52 board to the network on every probe, cached or not.
	var imgPath string
	for _, format := range []string{"bin", "uf2"} {
		p := emulated.BoardImagePath(cacheDir, emulated.BoardImage{
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
		// The tag this image lives under, rather than every release in the
		// repository. Upstream tags per role, so the one wanted here is
		// derivable from the version - and asking for it directly is one
		// request against a hundred.
		//
		// This is not a tidy-up. Listing every release takes about nine
		// seconds against GitHub and times out often enough to have failed
		// five boards in one sweep, reported as "could not reach the firmware
		// catalogue" against boards that were never the problem. Fetching the
		// single tag takes under half a second.
		all, err := cat.List(ctx, "repeater-"+version)
		if err != nil || len(all) == 0 {
			// A tag that is not named the way this expects is a reason to look
			// wider, not to give up: older releases and any future renaming
			// still resolve, just slowly.
			all, err = cat.ListAll(ctx)
		}
		if err != nil {
			report.set(Build, Failed, "could not reach the firmware catalogue: "+err.Error())
			return report
		}
		// Through Runnable, because a release carries more than one asset per
		// board and only some of them are a flash image. Taking the last match
		// picked the DFU .zip over the .uf2, which then got loaded as if it
		// were flash.
		var img emulated.BoardImage
		for _, i := range emulated.Runnable(all, nil) {
			// Case-insensitively, because a board profile's name and the
			// asset's differ in case more often than not - upstream publishes
			// Generic_E22_sx1262 beside Heltec_v3 - and an exact comparison
			// reports a board nobody has an image for, which is a different
			// and much more alarming thing than a spelling difference.
			if strings.EqualFold(i.Board, board) && i.Version == version && i.Role == "simple_repeater" {
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
	e := engine.New(flatEarth{}, engine.Config{
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

	// Read at the end whatever the probe does in between, because a hang can
	// happen after any step and there are a dozen ways out of this function.
	// The log is a file the emulator has already written, so it survives the
	// node being stopped and can be read here rather than at each return.
	//
	// Two files, because the two verdicts read two different voices. A boot
	// loop is the board saying the same first words over and over, and that is
	// on its serial port. A wedge is the emulator saying the board asked for an
	// address it does not implement, and that is the emulator's own output.
	// They shared a file until the emulator's noise started matching the
	// board's patterns.
	if said, ok := under.Firmware.Backend.(interface{ ConsoleLog() ([]byte, error) }); ok {
		defer func() {
			if log, err := said.ConsoleLog(); err == nil {
				report.downgradeIfRebooting(log)
			}
		}()
	}
	if said, ok := under.Firmware.Backend.(interface{ EmulatorLog() ([]byte, error) }); ok {
		defer func() {
			if log, err := said.EmulatorLog(); err == nil {
				report.downgradeIfWedged(log)
			}
		}()
	}

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
	txAt, outcome := waitForEvent(ctx, e, advertBudgetMs, func(ev engine.Event) bool {
		return ev.Kind == "tx" && ev.From == "bc-under-test"
	})
	switch outcome {
	case eventCancelled:
		report.set(Radio, Untested, cutShortDetail)
		report.set(TX, Untested, cutShortDetail)
		return report
	case eventTimedOut:
		never := fmt.Sprintf("never transmitted within %d s of coming up", advertBudgetMs/1000)
		report.set(Radio, Failed, never)
		report.set(TX, Failed, never)
		return report
	}
	report.set(Radio, Passed, fmt.Sprintf("adverted unprompted at %.1f s", float64(txAt)/1000))
	report.set(TX, Passed, fmt.Sprintf("adverted unprompted at %.1f s", float64(txAt)/1000))

	// rx: the sender adverts, the board under test should hear it.
	//
	// The board has to be off the air first, and this row did not wait. A tx
	// event is recorded at the start of a transmission - AtMs is now, endMs is
	// now plus the airtime - so the wait above returns while the board is still
	// sending its advert. Telling the sender to advert at that moment hands the
	// board a packet during its own transmission, and a half-duplex radio does
	// not hear it: the channel records a miss, the row's single stimulus is
	// gone, and nothing retries for the whole 240 s budget. The board was
	// behaving correctly and the physics was right; the test was handing it
	// something it could not have heard.
	//
	// The flood row below already learned this and waits. So does this one now.
	sender, ok := e.NodeByName("bc-sender")
	if ok && sender.Firmware != nil {
		if quiet, cutShort := waitUntilQuiet(ctx, e, "bc-under-test", floodQuietMs, advertBudgetMs); !quiet {
			msg := fmt.Sprintf(
				"the board never stopped transmitting for %d s, so it could not be "+
					"handed a packet it would hear", floodQuietMs/1000)
			if cutShort {
				msg = cutShortDetail
			}
			report.set(RX, Untested, msg)
			return report
		}
		// And the advert has to be one the board has not already seen. Every
		// node adverts once as it boots and the simulated clock does not move,
		// so a second advert from the sender is the same bytes as its first.
		// That does not stop the reception this row measures, which the channel
		// records before the frame reaches the firmware, but it does stop the
		// board acting on it - and a row that depends on the packet being new
		// should say so rather than rely on where the measurement is taken.
		//
		// An hour before the flood row's own timestamp, so that one is still
		// the newer packet when it runs.
		if err := setSenderClock(sender, 1754700000); err != nil {
			report.set(RX, Untested, "harness fault: could not set the sender's clock: "+err.Error())
			return report
		}
		_ = e.Run(ctx, e.NowMs()+1_000)
		// What the sender put on the air, and what the channel recorded arriving
		// at the board but not decoded - tracked exactly as the flood row tracks
		// them, so a silent rx row can say which of three things happened rather
		// than blaming the board for all of them. "no reception observed" read
		// the same whether the sender never spoke, the packet was too weak to
		// demodulate, or the board genuinely dropped one it heard - and the first
		// two are not the board's doing.
		fromSender := map[uint64]bool{}
		misses := map[uint64]string{}
		if err := sender.Firmware.Bridge.Type([]byte("advert\r\n")); err == nil {
			_, rxOutcome := waitForEvent(ctx, e, advertBudgetMs, func(ev engine.Event) bool {
				switch {
				case ev.Kind == "tx" && ev.From == "bc-sender":
					// Recorded at the start of the transmission, so by the time the
					// matching rx or miss is seen in this same scan the id is known.
					fromSender[ev.MessageID] = true
				case ev.Kind == "miss" && ev.To == "bc-under-test":
					misses[ev.MessageID] = ev.Detail
				case ev.Kind == "rx" && ev.To == "bc-under-test" && fromSender[ev.MessageID]:
					return true
				}
				return false
			})
			switch rxOutcome {
			case eventMatched:
				report.set(RX, Passed, "heard the sender's advert")
			case eventCancelled:
				report.set(RX, Untested, cutShortDetail)
			default:
				state, detail := rxSilence(fromSender, misses)
				report.set(RX, state, detail)
			}
		} else {
			report.set(RX, Failed, "could not command the sender: "+err.Error())
		}
	} else {
		report.set(RX, Failed, "the native sender never came up")
	}

	// flood: the board forwards somebody else's packet, judged over several
	// attempts because the single stimulus can land while the board is
	// briefly off the air. A board that forwards at least half of them
	// floods. See probeFlood in floodphase.go.
	report = probeFlood(ctx, e, report, sender, ok)

	return probeBoardHardware(ctx, e, report, board, under, sender)
}
