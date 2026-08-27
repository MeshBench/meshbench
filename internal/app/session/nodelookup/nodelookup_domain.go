// Package nodelookup holds the node-finding verbs - nodes.search by name and
// nodes.near by position. Both read only the world snapshot, so they take no
// Sim; they are split out of internal/app/session and register from init.
package nodelookup

import (
	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func init() {
	session.RegisterDomain(func(st *state.Store, _ *session.Sim) { registerNodeSearch(st) })
	session.RegisterDomain(func(st *state.Store, _ *session.Sim) { registerNodesNear(st) })
}
