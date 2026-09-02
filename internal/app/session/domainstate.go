package session

import "github.com/MeshBench/meshbench/internal/app/state"

// DomainState decouples a split-out domain's state from the Sim struct.
//
// Study and boundary and the other early domains held no state of their own, so
// they reached the Sim through plain accessors. The domains that follow -
// experiment, companion, provisioning and the rest - each kept a typed field on
// Sim (an *experiment, a map of companion sessions), and a field of the domain's
// own type is exactly what a split-out package cannot leave on Sim: the struct
// would have to name the type, which means importing the package, which imports
// session back. A cycle, for a pointer.
//
// So the state moves off the struct and into here, keyed by a name the domain
// picks. The domain asks for its state with a maker that builds it the first
// time; session never names the type, and the domain type-asserts the value it
// gets back. mk runs at most once per key per Sim, under the lock, so two
// goroutines racing to first-touch a domain still share one instance.
func DomainState[T any](s *Sim, key string, mk func() T) T {
	s.domainStateMu.Lock()
	defer s.domainStateMu.Unlock()
	if s.domainState == nil {
		s.domainState = map[string]any{}
	}
	if v, ok := s.domainState[key]; ok {
		// Same key, same maker, same type every call - the map is written only
		// here, so this assert cannot fail.
		return v.(T) //nolint:forcetypeassert // invariant held by this function
	}
	v := mk()
	s.domainState[key] = v
	return v
}

// teardowns run when the engine is torn down, before it is replaced; ticks run
// once per store step, to re-describe a domain's live state into the snapshot.
// Between them and DomainState, a domain that held a typed field on Sim and was
// read by core lifecycle and tick code can move out entirely: its state lives
// in DomainState, its cleanup in a teardown, and its per-tick snapshot refresh
// in a tick - none of which makes core name the domain's type.
var (
	teardowns []func(*Sim)
	ticks     []func(*Sim, *state.World)
	// setupRebuilds re-describe the readiness page after a verb has changed an
	// answer it reports. A tick cannot carry this: ticks run inside the engine
	// step, and the readiness page matters most on a first run, where there is
	// no scenario and nothing stepping.
	setupRebuilds []func(*Sim, *state.World)
)

// RegisterTeardown adds a reset run when the engine is torn down; RegisterTick
// adds a per-step snapshot refresh. Both are called from a domain package's
// init, and both run in registration order.
func RegisterTeardown(f func(*Sim))           { teardowns = append(teardowns, f) }
func RegisterTick(f func(*Sim, *state.World)) { ticks = append(ticks, f) }

func runTeardowns(s *Sim) {
	for _, f := range teardowns {
		f(s)
	}
}

func runTicks(s *Sim, w *state.World) {
	for _, f := range ticks {
		f(s, w)
	}
}

// RegisterSetupRebuild adds a re-describe of the readiness page, called from the
// resources domain's init. The page is built there and the answers it reports on
// are changed here, and the two are on opposite sides of an import: resources
// reaches into session, so session cannot call back into it by name.
func RegisterSetupRebuild(f func(*Sim, *state.World)) {
	setupRebuilds = append(setupRebuilds, f)
}

// rebuildSetup re-describes the readiness page into w.
//
// Called by every verb that changes an answer the page reports, because the
// page has to follow the state rather than the moment somebody last opened it.
// Consent to download terrain is granted from three places - the page's own
// row, the switch in Configuration, and a script calling the verb - and only
// the first of them used to leave the page correct. The other two left it
// asking a question that had already been answered, with the download it asked
// for visibly running in the status bar underneath.
func rebuildSetup(s *Sim, w *state.World) {
	for _, f := range setupRebuilds {
		f(s, w)
	}
}
