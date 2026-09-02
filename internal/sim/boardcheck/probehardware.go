// The two rows that ask about the board's own hardware rather than about
// anything crossing the air: whether its front-end module was switched in to
// transmit, and whether it is still answering after sitting idle.
//
// Split from probe.go because that file reached its length limit, and this is
// the seam that leaves both halves whole: the rows above it are about a packet
// getting from one node to another, and these two are about the board alone.
package boardcheck

import (
	"context"
	"fmt"
	"github.com/MeshBench/meshbench/internal/firmware"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// probeBoardHardware fills the fem and power rows and returns the finished
// report.
//
// Handed what it needs rather than resolving it again: under and sender are
// the nodes Probe already has, and board is the name it was asked about. The
// power row needs the sender because a board with no console is asked over the
// air instead.
func probeBoardHardware(ctx context.Context, e *engine.Engine, report BoardReport,
	board string, under, sender *engine.Node) BoardReport {
	// fem: the board's front-end module is switched in when it transmits.
	//
	// Asked of the board rather than assumed of the family. This row used to
	// report "no front-end module modelled for this board's wiring" for every
	// board, which was false for the ones that have one: the Generic E22
	// carries a module on GPIO 13 and a profile that prices it, and the engine
	// has been reading the line's state at each transmission all along.
	//
	// The line is judged at the instant the board last transmitted, not at its
	// level now: a module is meant to be switched out while the board listens,
	// so the question only means anything about a board that has transmitted.
	prof, perr := hw.BoardByName(board)
	switch {
	case perr != nil || prof.FEM == nil:
		report.set(FEM, NotApplicable, "this board carries no front-end module")

	// A board can only be judged on a line the emulator is able to drive.
	// QEMUWiring carries the module's enable pin; RenodeWiring has no field for
	// one, so a Renode board's firmware can toggle the pin all it likes and
	// nothing downstream will ever see it. Reporting that as a failure blames
	// the board for the emulator's gap, which is the same lie as a green cell
	// nobody earned.
	case prof.QEMU == nil || prof.QEMU.FEM == 0:
		report.set(FEM, Untested,
			"this board has a front-end module and this emulator has no pin for it")

	default:
		switch under.Firmware.Bridge.Stats().FemAtTx {
		case firmware.FemIn:
			report.set(FEM, Passed, "the module was switched in to transmit")
		case firmware.FemOut:
			// What it costs is the gain forgone plus the loss taken, and for
			// these two boards it is one or the other: the E22's module is an
			// attenuator worth 25 dB and the t096's an amplifier worth 13.
			report.set(FEM, Failed, fmt.Sprintf(
				"the module was switched out while transmitting, costing %.0f dB",
				prof.FEM.TxGainDB+prof.FEM.TxLossDB))
		default:
			report.set(FEM, Untested,
				"the firmware never reported where it left the module's enable line")
		}
	}

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
		// No console, so ask over the air instead.
		//
		// A board booted under Renode has no console at all and cannot be
		// given one without modelling the nRF52840's USB device: its firmware
		// reads its command interface from Serial, which the Adafruit core
		// puts on USB CDC, and the platform models two UARTs and no USB.
		//
		// Relaying answers the question anyway, and answers it about the whole
		// firmware rather than one task: a board that forwards somebody else's
		// packet has a radio receiving, a mesh stack deciding and a radio
		// transmitting, after having been left alone. It is only available to
		// a board that passed flood, because a board that never relays cannot
		// be asked this way either.
		if report.Results[Flood].State != Passed {
			report.set(Power, Untested, "no way to ask it: "+err.Error())
			return report
		}
		if !ok || sender.Firmware == nil {
			report.set(Power, Untested, "the native sender never came up to ask with")
			return report
		}
		if err := setSenderClock(sender, 1754707200); err != nil {
			report.set(Power, Untested, "harness fault: could not set the sender's clock: "+err.Error())
			return report
		}
		_ = e.Run(ctx, e.NowMs()+1_000)
		if err := sender.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
			report.set(Power, Untested, "could not command the sender: "+err.Error())
			return report
		}
		atMs, powerOutcome := waitForEvent(ctx, e, advertBudgetMs, func(ev engine.Event) bool {
			return ev.Kind == "tx" && ev.From == "bc-under-test"
		})
		switch powerOutcome {
		case eventMatched:
			report.set(Power, Passed,
				fmt.Sprintf("relayed again at %.1f s, after a 15 s idle", float64(atMs)/1000))
		case eventCancelled:
			report.set(Power, Untested, cutShortDetail)
		default:
			report.set(Power, Failed,
				"relayed before a 15 s idle and not after it")
		}
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
