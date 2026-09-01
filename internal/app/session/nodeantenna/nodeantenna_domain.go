// Package nodeantenna holds the antenna verbs: what a node stands under, and
// which way it points. Split out of internal/app/session; it reaches the
// running Sim through the accessors session exports and registers its verbs
// from init.
//
// The model has always been able to represent a beam. Nothing outside it could
// build one, so every node in every scenario was an omni aimed nowhere, and the
// one question a planning tool exists to answer - where should this thing point
// - could not be asked.
package nodeantenna

import "github.com/MeshBench/meshbench/internal/app/session"

func init() { session.RegisterDomain(registerNodeAntenna) }
