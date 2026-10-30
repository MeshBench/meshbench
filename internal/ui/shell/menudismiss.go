// Closing an open menu without choosing anything from it.
package shell

import (
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
)

// menuDismissed reports whether the open menu was asked to close: Escape, or a
// press outside it. The press is taken by the scrim rather than passed on, so
// a click that closes a menu does not also act on the map or panel beneath.
func (sh *Shell) menuDismissed(gtx layout.Context) bool {
	gone := false
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			gone = true
		}
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &sh.menuScrim, Kinds: pointer.Press})
		if !ok {
			break
		}
		if pe, ok := ev.(pointer.Event); ok && pe.Kind == pointer.Press {
			gone = true
		}
	}
	// Presses on the box itself are drained and ignored: they are what keeps
	// a click on a heading or the padding from falling through to the scrim.
	for {
		if _, ok := gtx.Event(pointer.Filter{Target: &sh.menuBox, Kinds: pointer.Press}); !ok {
			break
		}
	}
	return gone
}
