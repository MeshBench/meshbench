// Input at a stated instant of simulated time.
//
// A verb that writes to a node - a console line, a companion frame - lands
// between engine steps, and while the clock is playing which step it lands
// before is decided by when the call arrived on the wall clock. The same
// script on a loaded machine then configures its mesh at different simulated
// instants, and everything after that diverges. That is how a native run came
// to depend on the host it ran on, with the firmware itself fully lockstepped.
//
// at_ms takes the wall clock out of it: the call is held and run when the
// engine reaches that instant, on whichever machine. A call without it still
// runs at once, and if the clock is playing the run says it can no longer be
// reproduced, rather than letting a diff of two runs be read as a finding.
package session

import (
	"fmt"
	"sort"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// inputVerbs are the verbs that write to a node's firmware and take at_ms.
var inputVerbs = map[string]bool{
	"console.type":          true,
	"console.cli":           true,
	"companion.connect":     true,
	"companion.send":        true,
	"companion.advert":      true,
	"companion.add_channel": true,
	"companion.scope":       true,
	"companion.refresh":     true,
	"companion.raw":         true,
	"companion.configure":   true,
}

// queuedInput is one held call.
type queuedInput struct {
	atMs   uint32
	seq    uint64
	verb   string
	params any
	run    state.Handler
}

// inputQueue holds calls until their instant, in the order they are due and,
// at one instant, the order they were made.
type inputQueue struct {
	items []queuedInput
	seq   uint64
}

func (q *inputQueue) add(atMs uint32, verb string, params any, run state.Handler) {
	q.seq++
	q.items = append(q.items, queuedInput{atMs: atMs, seq: q.seq, verb: verb, params: params, run: run})
	sort.SliceStable(q.items, func(i, j int) bool {
		if q.items[i].atMs != q.items[j].atMs {
			return q.items[i].atMs < q.items[j].atMs
		}
		return q.items[i].seq < q.items[j].seq
	})
}

// due removes and returns everything due at or before now.
func (q *inputQueue) due(now uint32) []queuedInput {
	n := 0
	for n < len(q.items) && q.items[n].atMs <= now {
		n++
	}
	out := q.items[:n:n]
	q.items = q.items[n:]
	return out
}

func (q *inputQueue) clear() { q.items, q.seq = nil, 0 }

// handleInput registers an input verb with at_ms.
//
// With at_ms in the future the call is queued and answered with what was
// queued; at or before now it is refused, because "now" is exactly the
// instant a playing clock cannot promise. Without at_ms it runs as it always
// has.
func (s *Sim) handleInput(st *state.Store, verb string, h state.Handler) {
	if !inputVerbs[verb] {
		panic(fmt.Sprintf("session: %s is not listed in inputVerbs", verb))
	}
	st.Handle(verb, func(w *state.World, p any) (any, error) {
		at, ok := namedNum(p, "at_ms")
		if !ok {
			if w.Playing && !s.replaying {
				s.taint(fmt.Sprintf("%s reached %s while the clock was playing, at whatever instant "+
					"it happened to arrive - give it at_ms to place it", verb, nodeOf(p)))
			}
			return h(w, p)
		}
		if s.eng == nil {
			return nil, ErrNoSimulation
		}
		// A node that does not exist will not exist then either, and a
		// refusal now is one a script sees; a failure when it comes due is a
		// line in the log.
		if n, ok := stringField(p, "node"); ok && n != "" {
			if _, found := s.eng.NodeByName(n); !found {
				return nil, noSuchNode(n)
			}
		}
		now := s.eng.NowMs()
		if at < 0 || uint32(at) <= now {
			return nil, fmt.Errorf("%s: at_ms %.0f is not after now (%d ms)", verb, at, now)
		}
		s.inputs.add(uint32(at), verb, withoutAtMs(p), h)
		return map[string]any{"queued": true, "verb": verb, "at_ms": uint32(at), "now_ms": now}, nil
	})
}

// fireDueInputs runs the held calls whose instant has come.
//
// Run with replaying set, so a console verb on a stopped clock does not step
// the engine itself to fetch its reply - this is already inside a step, and
// the reply arrives with the next one like anybody else's.
func (s *Sim) fireDueInputs(w *state.World) {
	if s.eng == nil || len(s.inputs.items) == 0 {
		return
	}
	s.replaying = true
	defer func() { s.replaying = false }()
	for _, in := range s.inputs.due(s.eng.NowMs()) {
		if _, err := in.run(w, in.params); err != nil {
			w.Say(fmt.Sprintf("%s at %.1f s failed: %v", in.verb, float64(in.atMs)/1000, err))
		}
	}
}

// taint records the first thing that made this run depend on the wall clock.
func (s *Sim) taint(why string) {
	if s.tainted == "" {
		s.tainted = why
	}
}

// settleForReply steps a stopped clock so a console reply lands, the way the
// console verbs always have - but not for a call being replayed from the
// queue, which is already between two steps.
func (s *Sim) settleForReply(w *state.World) {
	if w.Playing || s.replaying {
		return
	}
	for i := 0; i < 60; i++ {
		s.step(w)
	}
	w.NowMs = s.eng.NowMs()
}

// pausedNote is what a console adds under a command accepted while the clock
// is stopped (#521). The firmware's own "OK - Advert sent" is true from its
// side: it queued the packet. Nothing goes on the air until the engine steps,
// and only MeshBench knows the run is paused, so the note is ours.
const pausedNote = "the run is paused: anything this sends is queued and goes on the air when it plays"

// pausedFor is pausedNote when it applies to a command running now, or "". A
// call replayed from the input queue runs inside a step, so it is not waiting
// for one.
func (s *Sim) pausedFor(w *state.World) string {
	if w.Playing || s.replaying {
		return ""
	}
	return pausedNote
}

func withoutAtMs(p any) any {
	m, ok := p.(map[string]any)
	if !ok {
		return p
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if k != "at_ms" {
			out[k] = v
		}
	}
	return out
}

func nodeOf(p any) string {
	if n, ok := stringField(p, "node"); ok && n != "" {
		return n
	}
	return "a node"
}

// notReproducible is why running this again on the same seed would not put
// the same traffic at the same instants, or "" if it would: an emulated node,
// which the scenario shows, or something the wall clock decided during this
// run, which it does not.
func (s *Sim) notReproducible() string {
	if why := scenario.NotReproducible(s.nodes); why != "" {
		return why
	}
	if s.tainted != "" {
		return s.tainted
	}
	if s.eng != nil && s.eng.ExternalInputs() > 0 {
		return "a client outside the simulation wrote to a served companion, on the wall clock " +
			"rather than at an instant of the run"
	}
	return ""
}
