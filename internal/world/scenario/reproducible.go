// Whether a run of this scenario can be repeated.
//
// The rest of the simulator is built so that one seed gives one answer: the
// noise is counter-based, the boot stagger is derived from the seed, and a
// native node's clock is supplied by the engine's tick rather than read off the
// host. A node running inside an emulator is the one exception, and it is a
// structural one rather than an oversight. Its firmware is a published image
// with nothing in it that could receive a tick; what answers the engine is the
// chip model on this side of the socket, so the acknowledgement says the
// engine's message was handled and says nothing about where the guest had got
// to. The guest is meanwhile executing against its emulator's own clock, which
// runs on the host's, and how far it gets between two ticks is a question about
// this machine's load rather than about the scenario.
//
// So a scenario carrying one is not reproducible, and the only honest thing to
// do is say so wherever somebody is about to build a comparison on it. Stated
// here, once, so the verb, the sweep and the interface all say the same thing.
package scenario

import (
	"fmt"
	"strings"
)

// EmulatedNodes names every node whose firmware runs inside an emulator, in
// the order they appear.
func EmulatedNodes(nodes []Node) []string {
	var out []string
	for _, n := range nodes {
		if n.Firmware.Emulated() {
			out = append(out, n.Name)
		}
	}
	return out
}

// NotReproducible is why running these nodes twice on one seed will not give
// the same answer twice, or "" when it will.
//
// A sentence rather than a flag because the flag on its own invites the wrong
// conclusion. "Not reproducible" reads as a fault to be fixed or a run to be
// discarded; what is actually true is narrower and more useful, which is that
// the timing came from a clock the seed does not reach, so this run's instants
// may be compared with nothing but themselves.
func NotReproducible(nodes []Node) string {
	return NotReproducibleWith(EmulatedNodes(nodes))
}

// NotReproducibleWith is the same sentence built from the names alone, for a
// caller holding a rendering of the network rather than the network - the
// interface, which draws from a snapshot and would otherwise have to word this
// a second time and word it differently.
func NotReproducibleWith(em []string) string {
	if len(em) == 0 {
		return ""
	}
	return fmt.Sprintf("%s in an emulator, on the host's clock rather than the "+
		"run's, so the same seed does not put its traffic at the same instants "+
		"twice: this run's timings can be compared with nothing but themselves",
		emulatedPhrase(em))
}

// emulatedPhrase is the subject of that sentence: who is being talked about.
//
// Named rather than counted while there are few enough to read, because the
// first question anybody asks of "this run is not reproducible" is which node
// did it, and a count sends them looking through the node list for an answer
// that was already in hand.
func emulatedPhrase(em []string) string {
	switch len(em) {
	case 1:
		return em[0] + " runs"
	case 2:
		return em[0] + " and " + em[1] + " run"
	case 3:
		return strings.Join(em[:2], ", ") + " and " + em[2] + " run"
	}
	return fmt.Sprintf("%s and %d other nodes run",
		strings.Join(em[:2], ", "), len(em)-2)
}
