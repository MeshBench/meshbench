package session

import "github.com/MeshBench/meshbench/internal/app/state"

// journalInterfaceOnly is what a click does to a window and nothing else: no
// world changed, so a history of how the world got here is not the place for
// it. The workers' own callbacks are left out too, but they say so at their
// registration and are excluded from there - see excludeInternalFromJournal.
var journalInterfaceOnly = []string{"resource.licence.hide"}

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
	st.ExcludeFromJournal(journalInterfaceOnly...)
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

// excludeInternalFromJournal keeps the workers' own callbacks out of the
// history, from the one place that already knows which verbs those are.
//
// Called at the end of Register rather than from registerJournal, because it
// reads the registration and every verb has to be registered first. Two lists
// of the same set was one list too many: the journal's copy was maintained by
// hand and by a test that compared it against a documentation file.
func excludeInternalFromJournal(st *state.Store) {
	st.ExcludeFromJournal(st.InternalVerbs()...)
}
