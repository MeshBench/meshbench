// The refusals a caller acts on differently.
//
// Sentinels rather than prose, so a client can tell "no node named that" from
// "the workbench is closing" without matching a sentence. The sentence is
// still what gets shown - these wrap it rather than replace it, and every one
// of them is fmt.Errorf("...: %w") at the site so the prose stays local to the
// verb that knows the situation.
//
// Deliberately three. A taxonomy is a second thing to keep correct; these are
// the ones a script branches on, and everything else is a fault.
package session

import "errors"

var (
	// ErrNoInterface is a window verb in a session with no window. Headless,
	// almost always.
	ErrNoInterface = errors.New("no interface attached")
	// ErrNoSimulation is the right request in the wrong state: nothing
	// loaded, or nothing running to act on.
	ErrNoSimulation = errors.New("no simulation")
	// ErrNoSuchNode is a name this scenario has not got.
	ErrNoSuchNode = errors.New("no such node")
)
