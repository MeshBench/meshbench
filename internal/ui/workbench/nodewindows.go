// Every node window that is open, and the one goroutine each of them runs.
//
// Separate from the panel that draws one: this file is about which windows
// exist and how another goroutine asks one to come forward, and nothing here
// knows what a node window looks like.
package workbench

import (
	"image"

	"gioui.org/io/key"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// nodeWindows tracks which nodes have a window, so a second request raises
// rather than opening a duplicate.
type nodeWindows struct {
	*windowSet
}

func newNodeWindows() *nodeWindows {
	return &nodeWindows{windowSet: newWindowSet()}
}

// nodeWindowHooks is how a node window reaches the rest of the application.
//
// A struct rather than a seventh positional callback: six was already a list
// nobody could read at the call site, and the companion client needs one more
// that carries parameters rather than only a node name.
type nodeWindowHooks struct {
	onCommand    func(node, line string)
	onAction     func(action, node string)
	onCLI        func(node, line string)
	onServe      func(node, kind string)
	onOpenPacket func(id uint64)
	onDo         func(verb string, params any)
}

func (w *nodeWindows) openFor(node string, tab nodeTab,
	newTheme func() *theme.Theme, st *state.Store, h nodeWindowHooks) {
	// Already out there: recall it rather than doing nothing. A second press
	// used to return in silence, which is indistinguishable from a dead menu
	// entry - and for a layered window dragged out of reach, the recall is
	// the only way back.
	if !w.claim(node) {
		return
	}
	p := &nodeWindowPanel{node: node, OnCommand: h.onCommand, OnAction: h.onAction,
		OnCLI: h.onCLI, OnServe: h.onServe, OnOpenPacket: h.onOpenPacket,
		OnDo: h.onDo, Kind: kindOfNode(st, node)}
	p.tab = tab
	go runPopout(w.windowSet, node, "MeshBench - "+node,
		popoutSize{820, 620}, p, newTheme, st)
}

var _ = key.NameEscape
var _ = image.Pt

// kindOfNode reads what a node is from the current snapshot.
func kindOfNode(st *state.Store, node string) string {
	s := st.Snapshot()
	if s == nil {
		return ""
	}
	for i := range s.Nodes {
		if s.Nodes[i].Name == node {
			return s.Nodes[i].Kind
		}
	}
	return ""
}

// openOnTab is which tab a node window opens on. Console, except when a
// capture is being taken of one of the others - a tab cannot be reached from
// outside the application otherwise, and a screenshot of it is how the tab
// gets checked.
var openOnTab nodeTab
