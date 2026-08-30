// Building a session, once.
//
// ADR-0019 asks for a headless mode that runs the same verbs as the windowed
// one, and warns that "every verb needs to behave identically in both modes,
// or the harness tests something users never run". The cheapest way to keep
// that true is not a test - it is for there to be one place a session is
// built, so the two modes cannot be two constructions that drift.
//
// What the workbench then does that the headless command does not is attach a
// UI. That is the whole difference, and it is one call.
package session

import (
	"fmt"
	"os"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// Options are the few choices a caller makes before any verb runs.
type Options struct {
	// StepMs is how much simulated time one tick advances. Zero takes the
	// default, which is what both entry points use.
	StepMs uint32
	// UnverifiedWiring runs boards whose wiring nobody has watched boot.
	//
	// Set here rather than through its verb because the store's loop is not
	// running yet, and it has to be decided before a network loads either
	// way: that is when the engine is built, and the engine is what reads it.
	UnverifiedWiring bool
	// Headless says no interface will be attached, which session.hello
	// reports so a script can check once rather than discover it from twelve
	// refusals in a row.
	Headless bool
	// NoPrefs skips the machine's saved settings - the GPU choice, the cache
	// bound, where the cache lives. For a test, whose session must not depend
	// on whatever this machine happens to have chosen.
	NoPrefs bool
}

const defaultStepMs = 10

// Boot builds the store, the simulator, and every verb on it.
//
// It does not start the store's loop: the caller decides when, because the
// window wants a worker and a test wants control of it.
func Boot(o Options) (*state.Store, *Sim) {
	step := o.StepMs
	if step == 0 {
		step = defaultStepMs
	}
	st := state.New(step)
	sm := &Sim{}
	if !o.NoPrefs {
		if err := sm.LoadPrefs(); err != nil {
			// Before the store runs and before a window exists, so stderr is
			// the only place this can be said - and it has to be said
			// somewhere: unreadable settings and settings nobody ever chose
			// look identical from the inside.
			fmt.Fprintln(os.Stderr, err)
		}
	}
	if o.UnverifiedWiring {
		sm.RunUnverifiedWiring()
	}
	Mode = "workbench"
	if o.Headless {
		Mode = "headless"
	}
	Register(st, sm)
	return st, sm
}
