// Package updates holds the update verbs: whether the release feed may be
// asked, asking it, and fetching what it named onto the disk beside this build.
//
// Split out of internal/app/session for the same reason the others were, and
// with one rule of its own that the others do not need: nothing in here runs
// unless something asks it to. The simulation does not depend on any of it, a
// machine with no network loses nothing by never using it, and a check that
// happened because the application opened would be exactly the behaviour this
// was written to avoid.
package updates

import "github.com/MeshBench/meshbench/internal/app/session"

func init() {
	session.RegisterDomain(registerCheck)
	session.RegisterDomain(registerDownload)
}
