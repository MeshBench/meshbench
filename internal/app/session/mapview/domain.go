// Package mapview holds the map's own verbs: where the camera looks, what is
// drawn under it, what a click on it does, and where a dragged node lands.
//
// Split out of internal/app/session, which is where they were after ui.go
// outgrew the file limit. They reach the running Sim through the accessors
// session exports - the map asks for very little of it - and register from
// init so the session package need not import them.
package mapview

import (
	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func init() {
	session.RegisterDomain(register)
}

func register(st *state.Store, s *session.Sim) {
	registerMapCamera(st, s)
	registerMapView(st, s)
	registerMapGestures(st, s)
}
