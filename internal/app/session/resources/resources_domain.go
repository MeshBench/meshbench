// Package resources holds the runtime-resource verbs - listing what is
// downloaded and what it cost the disk, fetching and removing it, the licence
// inventory, and the readiness check that reads all of it back as one answer.
// Split out of internal/app/session; it reaches the running Sim through the
// accessors session exports.
package resources

import (
	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func init() {
	session.RegisterDomain(registerResources)
	session.RegisterDomain(registerSetup)
	session.RegisterSetupRebuild(rebuildSetup)
}

// rebuildSetup re-describes the readiness page from what is true now, for a
// verb elsewhere that has just changed one of the answers it reports.
//
// Only a page that has already been built is refreshed. The check walks the
// resource cache on disk, and a session that has never been asked for the page
// should not pay for one nobody has opened; the panel asks on its first draw,
// which is where an unbuilt page comes from.
func rebuildSetup(s *session.Sim, w *state.World) {
	if len(w.Setup) == 0 {
		return
	}
	if _, err := relistResources(s, w); err != nil {
		// The page keeps the rows it has rather than being emptied by a
		// listing that failed: a stale answer is worse than a fresh one and
		// better than none, and the caller's own verb is still what reports.
		return
	}
	w.Setup = setupGroups(s, w)
}
