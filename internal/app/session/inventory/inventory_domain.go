// Package inventory holds the read-only inventory verbs - listing nodes and the
// recent event tail, and dumping the event log. Split out of
// internal/app/session; it reads the world snapshot and needs no Sim state.
package inventory

import "github.com/MeshBench/meshbench/internal/app/session"

func init() { session.RegisterDomain(registerInventory) }
