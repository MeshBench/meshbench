// Package fleet holds the bulk node verbs - sending one command to the whole
// fleet and collecting the replies, and the fleet-wide settings (regions,
// flood policy) and deletion. Split out of internal/app/session; it reaches the
// running Sim through the accessors session exports and registers its verbs
// from init.
package fleet

import "github.com/MeshBench/meshbench/internal/app/session"

func init() { session.RegisterDomain(registerFleet) }
