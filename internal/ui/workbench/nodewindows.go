// Every node window that is open, and the one goroutine each of them runs.
//
// Separate from the panel that draws one: this file is about which windows
// exist and how another goroutine asks one to come forward, and nothing here
// knows what a node window looks like.
package workbench

import (
	"image"
	"sync"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/float"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// nodeWindows tracks which nodes have a window, so a second request raises
// rather than opening a duplicate.
type nodeWindows struct {
	mu   sync.Mutex
	open map[string]bool
	// raising is which windows have been asked to come forward: a wish
	// rather than an action, because a window belongs to its own event
	// loop and reaching into it from another goroutine is how a destroyed
	// window stays in Gio's queue.
	raising map[string]bool
}

func newNodeWindows() *nodeWindows {
	return &nodeWindows{open: map[string]bool{}}
}

// raise asks a node's window to come back on screen on its next frame -
// which is also the escape hatch for one dragged somewhere its bar cannot
// be reached from, where raising alone would mean nothing.
func (w *nodeWindows) raise(node string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.raising == nil {
		w.raising = map[string]bool{}
	}
	if w.open[node] {
		w.raising[node] = true
	}
}

// wantsRaise reports and clears the wish, on the window's own goroutine.
func (w *nodeWindows) wantsRaise(node string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.raising[node] {
		delete(w.raising, node)
		return true
	}
	return false
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
	w.mu.Lock()
	if w.open[node] {
		// Already out there: recall it rather than doing nothing. A second
		// press used to return in silence, which is indistinguishable from
		// a dead menu entry - and for a layered window dragged out of
		// reach, the recall is the only way back.
		w.mu.Unlock()
		w.raise(node)
		return
	}
	w.open[node] = true
	w.mu.Unlock()

	go func() {
		defer func() {
			w.mu.Lock()
			delete(w.open, node)
			w.mu.Unlock()
		}()
		th := newTheme()
		p := &nodeWindowPanel{node: node, OnCommand: h.onCommand, OnAction: h.onAction,
			OnCLI: h.onCLI, OnServe: h.onServe, OnOpenPacket: h.onOpenPacket,
			OnDo: h.onDo, Kind: kindOfNode(st, node)}
		p.tab = tab
		spot := float.NextSpot()
		win := new(app.Window)
		// Whether it stays above the main window is the machine's preference,
		// read once here because the ask only exists at creation.
		win.Option(append([]app.Option{
			app.Title("MeshBench - " + node),
			app.Size(unit.Dp(820), unit.Dp(620)),
		}, float.Above(spot, keepAbove(st))...)...)
		// Raised as it opens, for the platforms where above is not or cannot
		// be honoured. Where it was, the window is on the overlay layer and
		// raising is meaningless anyway.
		win.Perform(system.ActionRaise)
		var chrome *layerChrome
		var ops op.Ops
		for {
			switch e := win.Event().(type) {
			case app.ConfigEvent:
				if e.Config.LayerShell && chrome == nil {
					p.Layered, chrome = true, newLayerChrome(spot)
				}
				if chrome != nil {
					chrome.outputSize(e.Config.OutputSize)
				}
			case app.DestroyEvent:
				return
			case app.FrameEvent:
				if w.wantsRaise(p.node) {
					// Raising means nothing to a layer surface, so for a
					// layered window the wish recalls it on screen instead -
					// which is also how one dragged out of reach comes back.
					if chrome != nil {
						if opts := chrome.recall(float.NextSpot()); len(opts) > 0 {
							win.Option(opts...)
						}
					} else {
						win.Perform(system.ActionRaise)
					}
				}
				gtx := app.NewContext(&ops, e)
				comp.Fill(gtx, th.P.Ground)
				if chrome != nil {
					chrome.frame(e)
				}
				layout.UniformInset(th.Sp.M).Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return p.Draw(th, gtx, st.Snapshot())
					})
				if chrome != nil {
					opts, close := chrome.update(&p.bar)
					p.maximised = chrome.maximised
					if close {
						win.Perform(system.ActionClose)
					} else if len(opts) > 0 {
						win.Option(opts...)
					}
				}
				e.Frame(gtx.Ops)
				win.Invalidate()
			}
		}
	}()
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
