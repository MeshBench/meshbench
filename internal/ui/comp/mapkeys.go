package comp

import (
	"gioui.org/io/key"
	"gioui.org/layout"
)

// The keyboard over the map.
//
// None of these filters names a focus target, which is how Gio says "only when
// nothing else wants it". That is the behaviour that matters here: typing a
// node's name into the search box must not delete the node.

// keys handles one frame's key presses. Called from handle, before the
// pointer, so a gesture and a shortcut cannot both act on the same frame.
func (m *MapView) keys(gtx layout.Context, pts []projected) {
	// Escape abandons the link tool's half-made pick, and tells the
	// workbench so - a pinned pair is released the same way.
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			m.CancelLink()
		}
	}

	// Delete removes what is selected, which is what the key means everywhere
	// else. Backspace as well: on a Mac the key labelled delete is the one Gio
	// calls DeleteBackward, and a shortcut that works on one keyboard and not
	// another reads as a shortcut that does not work.
	//
	// The map only reports what was asked for. Whether it happens, and whether
	// the operator is asked first, belongs to whoever knows what a node costs
	// to put back.
	for {
		ev, ok := gtx.Event(
			key.Filter{Name: key.NameDeleteForward},
			key.Filter{Name: key.NameDeleteBackward})
		if !ok {
			break
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press || m.OnDelete == nil {
			continue
		}
		var names []string
		for _, p := range pts {
			if p.n.Selected {
				names = append(names, p.n.Name)
			}
		}
		if len(names) > 0 {
			m.OnDelete(names)
		}
	}
}
