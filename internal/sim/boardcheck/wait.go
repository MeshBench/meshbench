// Waiting for a board to do something, on the clock the board actually runs on.
package boardcheck

import (
	"context"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

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

// waitUntilQuiet runs the engine on until the named node has put nothing on
// the air for quietMs, or the budget runs out.
//
// A packet that arrives while a board is transmitting is not received - the
// channel models half duplex, and the ledger records it as a miss. That is
// correct, and it silently ruins any phase that hands a board a stimulus
// without checking whether it is busy: the board never hears the thing it is
// being judged on, and gets marked down for not forwarding it.
//
// Returns whether the node actually went quiet. A caller that ignores that is
// back where it started.
func waitUntilQuiet(ctx context.Context, e *engine.Engine, node string,
	quietMs, budgetMs uint32) bool {

	deadline := e.NowMs() + budgetMs
	for e.NowMs() < deadline {
		last := uint32(0)
		for _, ev := range e.Events() {
			if ev.Kind == "tx" && ev.From == node && ev.AtMs > last {
				last = ev.AtMs
			}
		}
		if e.NowMs() >= last+quietMs {
			return true
		}
		if _, ok := waitForEvent(ctx, e, quietMs, func(engine.Event) bool { return false }); ok {
			return false
		}
		if ctx.Err() != nil {
			return false
		}
	}
	return false
}
