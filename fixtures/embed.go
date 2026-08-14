// Package fixtures carries the example networks inside the binary.
//
// They are embedded because MeshBench is installed, not unpacked into a
// working directory: a .deb puts its data in /usr/share, a macOS bundle puts
// it in Contents/Resources, and both are launched from a directory that has
// no fixtures in it. An application whose default network only opens when the
// shell happens to be in the right place is one that opens empty for most
// people who install it - which is exactly what 0.0.1 did.
//
// The files stay on disk as well. They are the ones people edit, copy and
// pass around, and the packages still ship them; this is the copy that cannot
// go missing.
package fixtures

import "embed"

// FS holds every shipped fixture, by file name.
//
//go:embed *.json
var FS embed.FS
