// Every node window that is open, and the one goroutine each of them runs.
//
// Separate from the panel that draws one: this file is about which windows
// exist and how another goroutine asks one to come forward, and nothing here
// knows what a node window looks like.
package nodeview

import (
	"image"

	"gioui.org/io/key"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// WindowSet tracks which nodes have a window, so a second request raises
// rather than opening a duplicate.
type WindowSet struct {
	*shell.WindowRegistry
}

func NewWindowSet() *WindowSet {
	return &WindowSet{WindowRegistry: shell.NewWindowRegistry()}
}

// WindowHooks is how a node window reaches the rest of the application.
//
// A struct rather than a seventh positional callback: six was already a list
// nobody could read at the call site, and the companion client needs one more
// that carries parameters rather than only a node name.
type WindowHooks struct {
	OnCommand    func(node, line string)
	OnAction     func(action, node string)
	OnCLI        func(node, line string)
	OnServe      func(node, kind string)
	OnOpenPacket func(id uint64)
	OnDo         func(verb string, params any)
}

func (w *WindowSet) OpenFor(node string, tab Tab,
	newTheme func() *theme.Theme, st *state.Store, h WindowHooks) {
	// Already out there: recall it rather than doing nothing. A second press
	// used to return in silence, which is indistinguishable from a dead menu
	// entry - and for a layered window dragged out of reach, the recall is
	// the only way back.
	if !w.Claim(node) {
		return
	}
	p := &WindowPanel{Node: node, OnCommand: h.OnCommand, OnAction: h.OnAction,
		OnCLI: h.OnCLI, OnServe: h.OnServe, OnOpenPacket: h.OnOpenPacket,
		OnDo: h.OnDo, Kind: kindOfNode(st, node)}
	p.Tab = tab
	go shell.RunPopout(w.WindowRegistry, shell.Popout{
		Key: node, Title: "MeshBench - " + node, Bar: node, W: 820, H: 620,
	}, p, newTheme, st)
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

// OpenOnTab is which tab a node window opens on. Console, except when a
// capture is being taken of one of the others - a tab cannot be reached from
// outside the application otherwise, and a screenshot of it is how the tab
// gets checked.
var OpenOnTab Tab
