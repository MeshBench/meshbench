// Package nodewipe holds node.wipe - deleting one node's stored settings and
// card so it next boots factory-fresh. Split out of internal/app/session; it
// reaches the running Sim through the accessors session exports.
package nodewipe

import "github.com/MeshBench/meshbench/internal/app/session"

func init() { session.RegisterDomain(registerNodeWipe) }
