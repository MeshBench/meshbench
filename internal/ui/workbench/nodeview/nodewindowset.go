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

// OpenFor opens or recalls a node's window and answers the tab it settles on,
// which is not always the tab asked for: a node whose board declares nothing
// grows no Hardware tab, an observer has no console, and a request for one the
// node has not got lands on the first tab it does have.
//
// Settled here rather than left to the first frame, so the answer exists
// before the window has drawn. It costs building the panel a moment early;
// the alternative is reporting the request back as though it were the outcome,
// which is what this used to do.
func (w *WindowSet) OpenFor(node string, tab Tab,
	newTheme func() *theme.Theme, st *state.Store, h WindowHooks) Tab {
	p := &WindowPanel{Node: node, OnCommand: h.OnCommand, OnAction: h.OnAction,
		OnCLI: h.OnCLI, OnServe: h.OnServe, OnOpenPacket: h.OnOpenPacket,
		OnDo: h.OnDo, Kind: kindOfNode(st, node)}
	// The same question the frame asks, from the same snapshot, through the
	// same functions - so the tab reported here is the tab that draws rather
	// than a second opinion that can drift from it.
	p.hasHardware = p.boardPanel(st.Snapshot()).HasAnything()
	p.Tab = settleTab(tab, p.visibleTabs())
	// Already out there: recall it rather than doing nothing. A second press
	// used to return in silence, which is indistinguishable from a dead menu
	// entry - and for a layered window dragged out of reach, the recall is
	// the only way back.
	//
	// A recalled window keeps the tab it is on, which may be one somebody
	// clicked to since. What comes back is what this request settled on, and
	// the verb's own description says which of the two it is.
	if !w.Claim(node) {
		return p.Tab
	}
	go shell.RunPopout(w.WindowRegistry, shell.Popout{
		Key: node, Title: "MeshBench - " + node, Bar: node, W: 820, H: 620,
	}, p, newTheme, st)
	return p.Tab
}

// settleTab is the tab a window showing these tabs lands on when asked for
// want: want itself where it is offered, and the first one otherwise.
//
// One rule rather than a list of special cases, and it happens to be every
// case the frame handles: a companion's set begins with Companion, an
// observer's with SDR, and everything else with Console, which is exactly
// where each of them was sending an impossible request by hand.
func settleTab(want Tab, tabs []Tab) Tab {
	for _, t := range tabs {
		if t == want {
			return want
		}
	}
	if len(tabs) == 0 {
		return want
	}
	return tabs[0]
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
