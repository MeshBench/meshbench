package boardcheck

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// measurePower fills the power row: is the board still running after being
// left alone?
//
// Its own file because it is the only capability that asks two different
// questions depending on what the board can be reached through, and reading
// Probe should not mean reading both of them.
func measurePower(ctx context.Context, e *engine.Engine, report *BoardReport,
	under, sender *engine.Node, ok bool) {

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
		return
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
		return
	}
	// Established before the idle, not assumed: a board that answers nothing on
	// its console cannot be measured this way, and calling that a failure would
	// mark a healthy board dead for saying little. Only a board that answered
	// before and does not after has actually stopped.
	baseline, err := said.ConsoleLog()
	if err != nil {
		report.set(Power, Untested, "could not read the console: "+err.Error())
		return
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
		// a board that passed flood, because a relay is the strongest answer
		// available without a console.
		//
		// A board that does not relay still gets asked, just a weaker question:
		// is it transmitting at all after the idle? Its own advert timer
		// answers that, and a timer is exactly the thing the flood row had to
		// stop accepting - but the two rows want different facts. Flood asks
		// whether the mesh stack decided something, where a timer is an
		// impostor. This row asks whether the firmware is still running, and a
		// timer that still fires is proof it is, because a stopped board has no
		// timers.
		//
		// The distinction is only sound now that the two can be told apart. The
		// detail says which question was answered, because "alive" and
		// "relaying after an idle" are not the same claim.
		if report.Results[Flood].State != Passed {
			if !ok || sender.Firmware == nil {
				report.set(Power, Untested, "no way to ask it: "+err.Error())
				return
			}
			if atMs, spoke := waitForEvent(ctx, e, advertBudgetMs, func(ev engine.Event) bool {
				return ev.Kind == "tx" && ev.From == "bc-under-test"
			}); spoke {
				report.set(Power, Passed, fmt.Sprintf(
					"still transmitting at %.1f s, after a 15 s idle - its own advert, "+
						"not a relay, so this shows the firmware running and not the mesh "+
						"stack deciding", float64(atMs)/1000))
			} else {
				report.set(Power, Failed, fmt.Sprintf(
					"silent for %d s after a 15 s idle, having transmitted before it - "+
						"a board with a two-minute advert timer that has stopped firing "+
						"has stopped", advertBudgetMs/1000))
			}
			return
		}
		if !ok || sender.Firmware == nil {
			report.set(Power, Untested, "the native sender never came up to ask with")
			return
		}
		_ = sender.Firmware.Bridge.Type([]byte("time 1754707200\r\n"))
		_ = e.Run(ctx, e.NowMs()+1_000)
		if err := sender.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
			report.set(Power, Untested, "could not command the sender: "+err.Error())
			return
		}
		// The sender's message, not any transmission - the same discrimination
		// the flood row makes, and for the same reason: this row exists to tell
		// a stopped board from a live one, and a two-minute advert timer will
		// answer for a board that has stopped deciding anything.
		again := map[uint64]bool{}
		if atMs, relayed := waitForEvent(ctx, e, advertBudgetMs, func(ev engine.Event) bool {
			if ev.Kind != "tx" {
				return false
			}
			if ev.From == "bc-sender" {
				again[ev.MessageID] = true
				return false
			}
			return ev.From == "bc-under-test" && again[ev.MessageID]
		}); relayed {
			report.set(Power, Passed,
				fmt.Sprintf("relayed again at %.1f s, after a 15 s idle", float64(atMs)/1000))
		} else {
			report.set(Power, Failed,
				"relayed before a 15 s idle and not after it")
		}
		return
	}
	if !answeredOn(ctx, e, said, len(baseline), "->") {
		report.set(Power, Untested,
			"this build answers nothing on its console, so silence later would "+
				"not mean it had stopped")
		return
	}

	before, err := said.ConsoleLog()
	if err != nil {
		report.set(Power, Untested, "could not read the console: "+err.Error())
		return
	}
	if err := under.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
		report.set(Power, Untested, "no way to ask it: "+err.Error())
		return
	}
	if answeredOn(ctx, e, said, len(before), "Advert sent") {
		report.set(Power, Passed, "answered a command on the console after a 15 s idle")
	} else {
		report.set(Power, Failed,
			"asked to advert after a 15 s idle and said nothing back on the console")
	}

	return
}
