// Package firmwarelib holds the firmware library: what builds exist on disk
// and upstream, what each node is pinned to, and the verbs that scan, download,
// build and pin them.
//
// Split out of internal/app/session because the library is a self-contained
// question - which images exist, and which one is this node running - that the
// session consults rather than owns. It reaches the running Sim through the
// accessors session exports, and registers its verbs from init so the session
// package need not import it.
//
// What did not come with it is the start gate. session's own
// firmwareStartBlocker asks whether a run can begin at all, and playready and
// runkind consult it, so it stays in core: core cannot import a package that
// imports core, and "may this run start" is the session's question anyway.
package firmwarelib

import (
	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func init() {
	session.RegisterDomain(register)
}

// domainKey is where this package's catalogue sits on a Sim.
const domainKey = "firmwarelib"

// catalogue is what the published-build fetch has come back with.
//
// Both fields were fields on Sim, which meant session declaring a type -
// publishedBuild - for something only this package reads. They are per-Sim
// state under session.DomainState now.
//
// No lock. Both are touched only on the store's goroutine, which is what makes
// them a single-flight guard rather than a race: the fetch runs elsewhere but
// hands its answer back through a verb, and the verb runs on the store's
// goroutine like every other.
type catalogue struct {
	// published is what the catalogue offers, fetched once: nil until the
	// fetch has answered, empty after a fetch that failed.
	published []publishedBuild
	fetching  bool
}

// catalogueOf is the one catalogue a Sim has, made on first use.
func catalogueOf(s *session.Sim) *catalogue {
	return session.DomainState(s, domainKey, func() *catalogue { return &catalogue{} })
}

func register(st *state.Store, s *session.Sim) {
	registerFirmwareWindow(st, s)
	registerFirmwareLibrary(st, s)
	registerFirmwareScan(st, s)
	registerFirmwareDetail(st, s)
	registerFirmwareBuild(st, s)
	registerFirmwareBuildResults(st, s)
}
