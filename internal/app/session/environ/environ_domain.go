// Package environ holds the environ.fetch verb - pulling building footprints at
// runtime from the OSM and Microsoft sources and ingesting them into the RF
// environment. Split out of internal/app/session so the download machinery and
// the orchestration are separately legible; it reaches the running Sim through
// the accessors session exports, and registers its verb from init so the
// session package need not import it.
package environ

import "github.com/MeshBench/meshbench/internal/app/session"

func init() {
	session.RegisterDomain(registerEnvironFetch)
}
