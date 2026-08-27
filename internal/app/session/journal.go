package session

import "github.com/MeshBench/meshbench/internal/app/state"

// journalWorkerCallbacks are the verbs a background goroutine calls back through
// the store to publish a result or report progress - a coverage raster is
// ready, a firmware process is up, a fetch failed. They are the process talking
// to itself, not a command anyone gave, so they are kept out of the journal.
//
// This is the same set facade.json marks no_facade (they have no client call
// because no client makes them); TestJournalSkipsEveryWorkerCallback holds the
// two together, so a new callback verb is excluded here the day it is added
// there.
var journalWorkerCallbacks = []string{
	"board.probe_finished",
	"coverage.failed", "coverage.set",
	"environ.failed", "environ.fetched",
	"experiment.finished",
	"feed.failed", "feed.set",
	"firmware.build_failed", "firmware.built", "firmware.failed", "firmware.started",
	"fleet.replies",
	"import.failed", "import.set",
	"infer.progress",
	"job.done", "job.progress",
	"link.pair_set", "link.profile_set", "links.set",
	"node.reflash_failed", "node.reflashed",
	"plan.failed", "plan.set",
	"resource.fetched", "resource.licence.hide",
	"terrain.cache_moved", "terrain.shade_failed", "terrain.shade_set",
	"validate.failed",
}

// journalPolls are the read-only verbs a script fires in a loop to wait for
// something. A hundred nodes.stats say nothing about what changed the world, so
// they are noise in a history rather than part of one. session.journal itself
// is here so reading the log does not write to it.
var journalPolls = []string{
	"sim.state", "sim.step",
	"nodes.stats", "nodes.list",
	"events.recent",
	"console.read",
	"board.screen",
	"node.output",
	"session.journal",
}

// registerJournal tells the store what not to journal, then registers the verb
// that reads it back. The exclusions are set here, from Register, which runs
// before the store does, so nothing is recorded that should not be.
func registerJournal(st *state.Store, _ *Sim) {
	st.ExcludeFromJournal(journalWorkerCallbacks...)
	st.ExcludeFromJournal(journalPolls...)

	st.HandleSpec("session.journal", state.Spec{
		What: "every command this workbench has been driven with, newest last, " +
			"and when the process started - so a session picked up cold can be " +
			"told how the world got here, and whether it has been restarted",
		Returns: []string{"started_ms", "count", "entries"},
	}, func(_ *state.World, _ any) (any, error) {
		startedMs, entries := st.Journal()
		return map[string]any{
			"started_ms": startedMs,
			"count":      len(entries),
			"entries":    entries,
		}, nil
	})
}
