// A node's kind, in the two forms the interface needs it.
//
// Both of these translate the kind string the store uses - "simple-repeater",
// "room-server" - into something a view can draw with: a short label, or the
// theme's own enum. They lived in menus.go, which is where the first caller
// happened to be, and are called from both the workbench and the node view.
//
// Here rather than in theme, which owns the colours and the sizes; a mapping
// from the application's vocabulary into that is a widget's business, not a
// palette's.
package comp

import (
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

func ShortKind(k string) string {
	switch k {
	case "simple-repeater":
		return "repeater"
	case "advanced-repeater":
		return "advanced"
	case "sdr-observer":
		return "observer"
	case "room-server":
		return "room server"
	}
	return k
}

func NodeKindOf(k string) theme.NodeKind {
	switch k {
	case "companion":
		return theme.Companion
	case "room-server":
		return theme.RoomServer
	case "sdr-observer":
		return theme.Observer
	case "emitter":
		return theme.Emitter
	case "advanced-repeater":
		return theme.AdvancedRepeater
	}
	return theme.SimpleRepeater
}
