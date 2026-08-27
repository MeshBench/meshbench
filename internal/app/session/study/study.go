// Package study holds the verbs that ask questions of a running simulation -
// coverage, planning, and the validation of predictions against what was
// heard. Split out of internal/app/session so that the orchestration and the
// studies built on it are separately legible; it reaches the running Sim
// through the accessors session exports, and registers its verbs from init so
// the session package need not import it.
package study

import "github.com/MeshBench/meshbench/internal/app/session"

func init() {
	session.RegisterDomain(registerPlanningVerbs)
	session.RegisterDomain(registerCoverageCombined)
	session.RegisterDomain(registerValidate)
	session.RegisterDomain(registerCoverageVerbs)
	session.RegisterDomain(registerCoverageMap)
}
