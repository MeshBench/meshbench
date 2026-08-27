// Package resources holds the runtime-resource verbs - listing what is
// downloaded and what it cost the disk, fetching and removing it, and the
// licence inventory. Split out of internal/app/session; it reaches the running
// Sim through the accessors session exports.
package resources

import "github.com/MeshBench/meshbench/internal/app/session"

func init() { session.RegisterDomain(registerResources) }
