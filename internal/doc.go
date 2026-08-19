// Package internal is not code. It exists so that the layer rule below has
// somewhere to live, next to the tree it describes rather than inside one of
// the layers it governs.
//
// Everything the application is made of sits under internal/ in seven layers:
//
//	rf → mesh → world → sim → study → app → ui
//
//	rf     radio physics; knows nothing of nodes, networks or the application
//	mesh   MeshCore itself: what a node is and what it says
//	world  what is being simulated, and where it came from
//	sim    running it, and recording what happened
//	study  the questions asked of a simulation
//	app    orchestration, with no user-interface toolkit
//	ui     Gio, and the only layer permitted one
//
// A package may import its own layer and everything beneath it. Nothing may
// import upward, which is what makes "ui can reach the physics, the physics
// cannot reach a widget" a property of the build rather than an intention.
// layers_test.go fails if that stops being true, and again if a package appears
// under internal/ outside the seven.
//
// The order was not designed and then imposed; it was read off the import graph
// that already existed. Making it true cost two packages that were each doing
// two jobs - internal/coverage and internal/capture - and one interface moved
// down a level. Nothing was bent to fit, which is the reason to trust it.
package internal
