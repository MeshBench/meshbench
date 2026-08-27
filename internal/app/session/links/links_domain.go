// Package links holds the link inspection verbs - pairing two points and
// cutting a terrain profile between them (link.pair, link.profile and their
// set counterparts). Split out of internal/app/session; profileFor stays in
// core because the link budget shares it, reached here through ProfileFor.
package links

import "github.com/MeshBench/meshbench/internal/app/session"

func init() {
	session.RegisterDomain(registerLinkPair)
	session.RegisterDomain(registerLinkProfile)
}
