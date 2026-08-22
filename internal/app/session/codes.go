// Which errors mean which code on the wire.
//
// Registered here rather than where the errors are defined, because control is
// the bottom of the tree and cannot import upwards. Registration rather than
// pattern-matching on the message: a code inferred from prose breaks the
// moment somebody improves the prose, which is the coupling the codes exist to
// remove.
package session

import (
	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func init() {
	// The store's own, which control cannot see and this package cannot
	// change.
	control.Classify(state.ErrUnknownVerb, control.UnknownVerb)
	control.Classify(state.ErrStopped, control.Closing)
	// Ours, returned bare rather than wrapped - see errors.go for why the
	// others carry their code at the site instead.
	control.Classify(ErrNoSimulation, control.Conflict)
}
