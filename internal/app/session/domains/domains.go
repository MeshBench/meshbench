// Package domains blank-imports the verb packages split out of session, so a
// program that wants their verbs pulls them all in with one import rather than
// tracking each. Every such package registers its verbs from its own init;
// importing it here is what makes that init run. session itself must not import
// them - they import it - so this aggregator lives beside session, not in it.
package domains

import (
	_ "github.com/MeshBench/meshbench/internal/app/session/boundary"
	_ "github.com/MeshBench/meshbench/internal/app/session/environ"
	_ "github.com/MeshBench/meshbench/internal/app/session/fleet"
	_ "github.com/MeshBench/meshbench/internal/app/session/study"
	_ "github.com/MeshBench/meshbench/internal/app/session/sweep"
)
