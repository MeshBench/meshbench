// Package boundary holds the study-area verbs - searching for a place, taking
// one into the study, loading one from GeoJSON, and pruning the scenario back
// to it. Split out of internal/app/session so the orchestration and the study
// area it decides are separately legible; it reaches the running Sim through
// the accessors session exports, and registers its verbs from init so the
// session package need not import it.
package boundary

import (
	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func init() {
	session.RegisterDomain(registerBoundary)
	session.RegisterDomain(registerBoundaryLoad)
	// boundary.list reads only the world, so it takes no Sim; adapt it to the
	// registrar's shape.
	session.RegisterDomain(func(st *state.Store, _ *session.Sim) { registerBoundaryList(st) })
}
