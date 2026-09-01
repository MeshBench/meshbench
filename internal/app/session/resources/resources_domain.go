// Package resources holds the runtime-resource verbs - listing what is
// downloaded and what it cost the disk, fetching and removing it, the licence
// inventory, and the readiness check that reads all of it back as one answer.
// Split out of internal/app/session; it reaches the running Sim through the
// accessors session exports.
package resources

import "github.com/MeshBench/meshbench/internal/app/session"

func init() {
	session.RegisterDomain(registerResources)
	session.RegisterDomain(registerSetup)
}
