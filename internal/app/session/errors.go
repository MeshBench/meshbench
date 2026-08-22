// The refusals a caller acts on differently, and how they are classified.
//
// Two mechanisms, for two situations.
//
// Where the error is raised here, the code goes on at the site and the
// sentence is left exactly as it was. Wrapping a sentinel with %w was the
// first attempt and it stuttered - "no node named \"Nowhere\": no such node" -
// which is a worse message than the one it replaced, for the benefit of a
// caller that gets the same answer from the code.
//
// Where the error comes from somewhere this package does not own, the sentinel
// is registered in codes.go instead. Never by matching the message: a code
// inferred from prose breaks the moment somebody improves the prose, which is
// the coupling all of this exists to remove.
package session

import (
	"errors"
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/control"
)

// ErrNoSimulation is the right request in the wrong state: nothing loaded, or
// nothing running to act on.
//
// A sentinel rather than a helper because it is returned bare - its own text
// is the whole message, so there is nothing for a wrap to stutter against.
var ErrNoSimulation = errors.New("no simulation")

// noSuchNode is a name this scenario has not got, worded the way every verb
// already worded it.
func noSuchNode(name string) error {
	return control.WithCode(control.NotFound, fmt.Errorf("no node named %q", name))
}
