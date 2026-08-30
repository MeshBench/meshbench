// What a node is while a run is in progress, and the one rule for changing it.
//
// Split from the engine because the rule is the point: anything here that
// changes during a run is swapped whole through an atomic rather than edited
// field by field, so a reader holding a pointer taken under the lock never
// sees half a change.
package engine

import (
	"sync/atomic"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Node is one participant: its place in the world and its running firmware.
//
// The rule for everything on it, because the package's shape forces one:
// *Node pointers are copied out under e.mu and then dereferenced long after
// it is released. deliver, the observers and the carrier-sense scans all
// snapshot the slice and work outside the lock, because pathLoss takes the
// same mutex and Go's is not reentrant. So the lock orders the writes; it is
// not what makes the reads safe. Anything here that changes while a run is in
// progress is therefore swapped whole through an atomic rather than edited
// field by field - which is why the spec is behind an accessor. The counters
// are the exception that proves it: they are written under e.mu and read
// under it, by Scoreboard, and never touched from a snapshot.
type Node struct {
	// state is what the world says this node is, plus what was measured from
	// where it stands. Never edited in place: a change builds a whole new
	// value and stores it, so a reader holding the old one has a consistent
	// node rather than half of two. ApplyRadioState rewrites transmit power
	// and noise figure every tick, which is what makes this the field the
	// rule exists for.
	state atomic.Pointer[nodeState]

	// Firmware is nil for a node that does not run any — an SDR observer, or a
	// custom emitter that is only there to be interfered with.
	Firmware *firmware.Node

	// BootOffsetMs is how far ahead of the run clock this node's own clock runs,
	// standing in for having been powered on earlier than the others.
	BootOffsetMs uint32

	// The board's own figures, kept because Spec's are overwritten with the
	// effective ones as the firmware reports how it has configured its radio.
	// Without a baseline to compute from, every tick would apply the same
	// correction again to the previous tick's answer.
	baseTxPowerDBm, baseNoiseFigDB float64

	// Sent and Heard are counters for the scoreboard.
	Sent           int
	Heard          int
	UniqueDelivery int
	RedundantRelay int
	AirtimeMs      float64
}

// nodeState is one immutable snapshot of a node: the spec, and the ground
// under it once the terrain has been asked.
//
// The ground rides along because it is a property of where the node stands,
// so a change of position invalidates both at once and nothing has to
// remember to invalidate the second one. Asking the DEM is the expensive part
// - it can reach the network - and the look angle to every far end needs it.
type nodeState struct {
	spec scenario.Node
	// groundM is metres above sea level under the node; groundKnown says the
	// terrain has been asked, because sea level is a real answer.
	groundM     float64
	groundKnown bool
}

// Spec is what the world says this node is, as one consistent value.
//
// Safe from any goroutine, which is the whole point: the callers that matter
// hold a pointer taken under e.mu and read it long after.
func (n *Node) Spec() scenario.Node { return n.state.Load().spec }

// specRef is Spec without the copy, for the arithmetic that runs per receiver
// per transmission. The value behind it is never written, so the pointer is
// as safe as the copy and several times cheaper.
func (n *Node) specRef() *scenario.Node { return &n.state.Load().spec }

// changeSpec swaps a modified spec in. The caller holds e.mu, because the
// load, the change and the store are three steps and two writers racing
// through them would lose one of the changes.
//
// A node that moved loses its measured ground here rather than at each
// caller: the position and the terrain under it are one fact.
func (n *Node) changeSpec(f func(*scenario.Node)) {
	st := *n.state.Load()
	was := st.spec.Position
	f(&st.spec)
	if st.spec.Position != was {
		st.groundM, st.groundKnown = 0, false
	}
	n.state.Store(&st)
}
