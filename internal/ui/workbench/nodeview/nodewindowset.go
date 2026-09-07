// Every node window that is open, and the one goroutine each of them runs.
//
// Separate from the panel that draws one: this file is about which windows
// exist and how another goroutine asks one to come forward, and nothing here
// knows what a node window looks like.
package nodeview

import (
	"sync"

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
	mu sync.Mutex
	// wantTab is the tab a window already open has been asked to switch to.
	//
	// A wish rather than an action, like the registry's own raising and
	// closing and for the same reason: the window belongs to another event
	// loop, and writing its state from this goroutine is a data race. The
	// window takes it on its next frame.
	wantTab map[string]Tab
}

func NewWindowSet() *WindowSet {
	return &WindowSet{WindowRegistry: shell.NewWindowRegistry(),
		wantTab: map[string]Tab{}}
}

// askTab leaves a wish for an open window to change tab.
func (w *WindowSet) askTab(node string, tab Tab) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.wantTab == nil {
		w.wantTab = map[string]Tab{}
	}
	w.wantTab[node] = tab
}

// takeTab collects that wish, once.
func (w *WindowSet) takeTab(node string) (Tab, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	tab, ok := w.wantTab[node]
	delete(w.wantTab, node)
	return tab, ok
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
	p.set = w
	// Already out there: recall it rather than doing nothing. A second press
	// used to return in silence, which is indistinguishable from a dead menu
	// entry - and for a layered window dragged out of reach, the recall is
	// the only way back.
	//
	// The recalled window is also switched to the tab it was asked for. It
	// used to keep whatever tab it was on while this returned the tab that had
	// been requested, so a caller naming one was told it had what it asked for
	// and nothing moved. Every panel and section here is reachable by flag and
	// by verb precisely so a capture can reach it, and a window that ignores
	// the argument it was given cannot be driven onto a pane.
	if !w.Claim(node) {
		w.askTab(node, p.Tab)
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
