// The flood row: whether the board forwards somebody else's packet, judged
// over several attempts rather than one.
//
// An emulated node runs against the host clock, not the engine's
// (scenario.NotReproducible), so its receive window is not reproducible: a
// single flood stimulus can land in the moment the board is off the air after
// its own periodic advert (half duplex), and a board that forwards perfectly
// still misses it. One shot was flaky - about three passes in four for a board
// that floods. So this hands the board several fresh adverts and passes it when
// it forwards at least half.
package boardcheck

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// floodAttempts is how many fresh adverts the flood row hands the board. Kept
// small on purpose: simple_repeater forwards at most four adverts every two
// minutes (its discover_limiter), and the rx row already spent one forwarding
// the sender's advert, so more attempts than this would measure the rate limit
// rather than the board.
const floodAttempts = 2

// floodPassNum/floodPassDen is the share of attempts a board must forward to be
// judged as flooding: at least half.
const (
	floodPassNum = 1
	floodPassDen = 2
)

// floodAttemptMs bounds one attempt's wait for the relay. The board forwards
// within about a second of hearing an advert, so this is generous; five of
// them plus their quiet waits stay inside the two flood phases ProbeBudget
// already grants (quiet + relay), so the multi-attempt row costs no more
// context than the single-shot one did.
//
// "About a second" was measured on a board under QEMU. A Renode board runs its
// guest markedly slower against the same simulated clock - measured at roughly
// a quarter of the pace, first advert at 105 s of simulated time against 28.5 s
// - and every delay the firmware counts in its own milliseconds costs
// proportionally more of this budget. MESHBENCH_FLOOD_ATTEMPT_MS exists to find
// out whether a board that fails this row is refusing to forward or merely
// being timed against a clock it cannot keep up with; those are different
// faults and the row cannot tell them apart on its own.
var floodAttemptMs = envMs("MESHBENCH_FLOOD_ATTEMPT_MS", 90_000)

// floodQuietBudgetMs bounds one attempt's wait for the board to go quiet.
var floodQuietBudgetMs = envMs("MESHBENCH_FLOOD_QUIET_MS", 20_000)

// envMs reads a millisecond budget from the environment, or keeps the default.
// A value that is not a number is the default rather than a failure: a probe
// that dies over a typo in a diagnostic setting has lost the measurement it was
// there to take.
func envMs(name string, def uint32) uint32 {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return uint32(n)
}

// probeFlood measures whether the board forwards the sender's advert, over
// floodAttempts, and passes when it forwards at least half of the attempts it
// was actually given (one where the board never went quiet is not counted
// against it).
func probeFlood(ctx context.Context, e *engine.Engine, report BoardReport,
	sender *engine.Node, ok bool) BoardReport {
	if !ok || sender.Firmware == nil {
		report.set(Flood, Failed, "the native sender never came up")
		return report
	}
	// Two different nodes, before anything is asked of them.
	//
	// A repeater drops an advert it believes it sent itself, so a sender and a
	// board that share a public key make this row unmeasurable - and it fails
	// in exactly the way a board that refuses to forward fails, which is how
	// days get spent on the wrong layer. Emulated boards really did all share
	// one key: their radio handed the firmware the same "random" bytes, because
	// the model's random-number registers were backed by storage that read zero.
	if why := sameIdentity(e, sender); why != "" {
		report.set(Flood, Untested, why)
		return report
	}

	relayed, arrived, attempted := 0, 0, 0
	for i := 0; i < floodAttempts; i++ {
		// The board must be off the air first: a packet handed to a
		// transmitting half-duplex radio is a miss, not a fair test.
		quiet, cut := waitUntilQuiet(ctx, e, "bc-under-test", floodQuietMs, floodQuietBudgetMs)
		if cut {
			report.set(Flood, Untested, cutShortDetail)
			return report
		}
		if !quiet {
			// Never idle long enough this attempt - not the board declining to
			// forward, so it is not counted. Move to the next.
			continue
		}
		// A distinct timestamp per attempt, well past the rx row's own advert,
		// so each is a packet the board has not seen and cannot drop as a
		// duplicate.
		if err := setSenderClock(sender, 1754704000+int64(i)*3600); err != nil {
			report.set(Flood, Untested, "harness fault: could not set the sender's clock: "+err.Error())
			return report
		}
		_ = e.Run(ctx, e.NowMs()+1_000)

		fromSender := map[uint64]bool{}
		gotIt := false
		if err := sender.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
			report.set(Flood, Failed, "could not command the sender: "+err.Error())
			return report
		}
		attempted++
		_, out := waitForEvent(ctx, e, floodAttemptMs, func(ev engine.Event) bool {
			switch {
			case ev.Kind == "tx" && ev.From == "bc-sender":
				// The sender's own advert, identified by payload; a relay carries
				// this same id because MessageID hashes the payload, not the route.
				fromSender[ev.MessageID] = true
			case ev.Kind == "rx" && ev.To == "bc-under-test" && fromSender[ev.MessageID]:
				gotIt = true
			case ev.Kind == "tx" && ev.From == "bc-under-test" && fromSender[ev.MessageID]:
				return true // the board put the sender's advert back on the air
			}
			return false
		})
		switch out {
		case eventMatched:
			relayed++
			arrived++
		case eventCancelled:
			report.set(Flood, Untested, cutShortDetail)
			return report
		default:
			if gotIt {
				arrived++
			}
		}
		if os.Getenv("FLOODDBG") != "" {
			fmt.Fprintf(os.Stderr, "[flooddbg] attempt %d: out=%v gotIt=%v atMs=%d\n",
				i, out, gotIt, e.NowMs())
		}
	}

	switch {
	case attempted == 0:
		report.set(Flood, Untested,
			"the board never went quiet long enough to be handed an advert to forward")
	case relayed*floodPassDen >= attempted*floodPassNum:
		report.set(Flood, Passed, fmt.Sprintf(
			"forwarded the sender's advert in %d of %d attempts", relayed, attempted))
	case arrived > 0:
		report.set(Flood, Failed, fmt.Sprintf(
			"received the advert but forwarded it in only %d of %d attempts", relayed, attempted))
	default:
		report.set(Flood, Untested, fmt.Sprintf(
			"no evidence the advert reached the board in any of %d attempts: the ledger held "+
				"neither a reception nor a relay, which is what it shows when a packet is too "+
				"weak or on another preset", attempted))
	}
	return report
}

// sameIdentity reports why the flood row cannot be measured, or "".
//
// Best effort on purpose: a node whose identity cannot be read is not a reason
// to refuse to measure, only a reason not to claim the two were checked. What
// it must never do is stay quiet when they genuinely match.
func sameIdentity(e *engine.Engine, sender *engine.Node) string {
	under, ok := e.NodeByName("bc-under-test")
	if !ok || under.Firmware == nil {
		return ""
	}
	boardKey := consoleIdentity(under)
	senderKey := storedIdentity(sender)
	if boardKey == "" || senderKey == "" || !strings.EqualFold(boardKey, senderKey) {
		return ""
	}
	return "the board and the sender have the same public key (" + boardKey[:16] +
		"...), so the board would drop the advert as its own - this measures the " +
		"harness, not the board"
}

// consoleIdentity reads the public key a repeater prints as it comes up.
func consoleIdentity(n *engine.Node) string {
	said, ok := n.Firmware.Backend.(interface{ ConsoleLog() ([]byte, error) })
	if !ok {
		return ""
	}
	log, err := said.ConsoleLog()
	if err != nil {
		return ""
	}
	m := repeaterID.FindSubmatch(log)
	if m == nil {
		return ""
	}
	return string(m[1])
}

// storedIdentity reads the public key out of a native node's keypair file,
// which holds the private key, then the public key, then the display name.
func storedIdentity(n *engine.Node) string {
	has, ok := n.Firmware.Backend.(interface{ IdentityPath() string })
	if !ok {
		return ""
	}
	path := has.IdentityPath()
	if path == "" {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) < 64 {
		return ""
	}
	return fmt.Sprintf("%X", raw[32:64])
}

var repeaterID = regexp.MustCompile(`Repeater ID: ([0-9A-Fa-f]{64})`)
