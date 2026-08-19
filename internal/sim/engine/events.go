// The run's own record of what happened.
//
// Every reception, every miss and every transmission, in time order, with the
// reason attached. It is the only account of a run that survives it, so the
// readers here are careful about cost: a tick that asks for the whole log
// copies the whole log, which is quadratic over a run and was once the reason
// long runs slowed down.
package engine

import (
	"strings"

	"github.com/MeshBench/meshbench/internal/sim/capture"
)

// Event is one thing that happened, in simulated time.
//
// The timeline is built from these, and so is every explanation. A simulation
// that reports only final counts cannot answer "why did that not arrive", which
// is the only question anyone actually has.
type Event struct {
	AtMs     uint32
	Kind     string // "tx", "rx", "miss"
	From     string
	To       string
	PacketID uint64
	// MessageID is the same for every hop of one message, where PacketID is one
	// transmission. Following a message across a mesh needs the first; blaming
	// a particular relay needs the second.
	MessageID uint64
	Outcome   capture.Outcome
	SNRdB     float64
	Detail    string

	// Frame is the bytes on the air. Carried on the event so the inspector can
	// dissect what actually flew rather than a reconstruction of it — the two
	// diverge exactly when it matters, which is when something is wrong.
	//
	// Shared, not copied: the engine never mutates a frame after transmission,
	// and a hundred thousand events each owning a copy is real memory.
	Frame []byte
}

func (e *Engine) record(ev Event) {
	e.mu.Lock()
	e.events = append(e.events, ev)
	if e.classCounts == nil {
		e.classCounts = map[string]int{}
	}
	e.classCounts[EventClass(ev.Kind, ev.Detail)]++
	l := e.eventLog
	e.mu.Unlock()
	if l != nil {
		l.write(ev)
	}
}

// Events is everything that has happened, in order.
// EventCount is the ledger length, for callers deciding whether to resnapshot.
func (e *Engine) EventCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.events)
}

func (e *Engine) Events() []Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Event, len(e.events))
	copy(out, e.events)
	return out
}

// EventsTail copies only the last n events, and says how many there are.
//
// The tick asked for Events() and threw away all but the tail, which is a
// copy of the whole run's history per tick - quadratic over a run's life,
// and the reason a long run's ticks grew slow.
func (e *Engine) EventsTail(n int) ([]Event, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	total := len(e.events)
	if n > total {
		n = total
	}
	out := make([]Event, n)
	copy(out, e.events[total-n:])
	return out, total
}

// EventsSince copies only the events at or after a simulated moment. Events
// arrive in time order, so the start is found by binary search rather than by
// walking the whole log.
func (e *Engine) EventsSince(fromMs uint32) []Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	lo, hi := 0, len(e.events)
	for lo < hi {
		mid := (lo + hi) / 2
		if e.events[mid].AtMs < fromMs {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	out := make([]Event, len(e.events)-lo)
	copy(out, e.events[lo:])
	return out
}

// EventClass buckets an event by what happened to it, which is what the
// interface's cards and filter chips count: a miss lost to the node's own
// transmitter, a miss lost to a stronger signal, and a miss that was simply
// too quiet are three different problems with three different fixes.
func EventClass(kind, detail string) string {
	switch kind {
	case "tx":
		return "sent"
	case "rx":
		return "received"
	}
	// Prefixes, matching how the details above are written - and not
	// strings.Contains, which a guard test forbids in this package to keep
	// region logic out of the channel.
	switch {
	case strings.HasPrefix(detail, "its own transmitter"):
		return "half-duplex"
	case strings.HasPrefix(detail, "would have decoded"):
		return "interference"
	default:
		return "floor"
	}
}

// EventCounts is how many events of each class the run has produced, counted
// as they are recorded rather than by walking the log - the log is millions
// on a long run, and the cards asking for these ask every tick.
func (e *Engine) EventCounts() map[string]int {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[string]int, len(e.classCounts))
	for k, v := range e.classCounts {
		out[k] = v
	}
	return out
}
