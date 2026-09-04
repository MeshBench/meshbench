// Package experiment holds the A/B matrix: the arms, the seeds, the cells that
// run real MeshCore under each, and the report that comes back.
//
// Split out of internal/app/session because the matrix is a self-contained
// thing with its own type and its own lifetime - it is defined, run, stopped
// and reported on, and none of that is the session's business beyond starting
// it. It reaches the running Sim through the accessors session exports, and
// registers its verbs from init so the session package need not import it.
//
// What did not come with it is the arm: session.ExpArm stays in core because
// provisioning and the sweep verbs describe the same configuration under test,
// and a type two packages import cannot live inside either of them.
package experiment

import (
	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func init() {
	session.RegisterDomain(register)
}

// domainKey is where this package's matrix sits on a Sim.
const domainKey = "experiment"

// matrixOf is the one matrix a Sim has, made on first use.
//
// Through session.DomainState rather than a field on Sim, which is what let
// the matrix leave that struct at all: session never names this type, and the
// state still dies with the Sim that owns it. The maker runs at most once per
// Sim even if two goroutines reach it together.
func matrixOf(s *session.Sim) *experiment {
	return session.DomainState(s, domainKey, newExperiment)
}

// register hands the one matrix to each verb set.
//
// One registrar rather than four, because all four act on the same matrix:
// defining arms and reading results are one object seen from two verbs.
func register(st *state.Store, s *session.Sim) {
	e := matrixOf(s)
	registerExperiment(st, s, e)
	registerExperimentResults(st, s, e)
	registerExperimentControl(st, s, e)
	registerExperimentDone(st, s, e)
}
