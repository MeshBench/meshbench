// Which bring-up windows exist, and the goroutine each one runs.
//
// Separate from the panel that draws one, for the reason the node view's own
// window set is: this file is about which windows are open and how another
// goroutine asks one to come forward, and nothing here knows what the window
// looks like.
package boardview

import (
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// WindowSet tracks which nodes have a bring-up window open, so a second
// request raises the one that exists rather than opening a duplicate.
type WindowSet struct {
	*shell.WindowRegistry
}

func NewWindowSet() *WindowSet {
	return &WindowSet{WindowRegistry: shell.NewWindowRegistry()}
}

// Hooks is how a bring-up window reaches the rest of the application.
type Hooks struct {
	OnDo func(verb string, params any)
	// OnSaveShot asks for a file and writes the panel to it. Through the
	// application rather than the panel, because opening the platform's own
	// dialog is the shell's business.
	OnSaveShot func(node, suggested string)
}

// OpenFor opens the window for a node, or recalls the one already out there.
func (w *WindowSet) OpenFor(node string, newTheme func() *theme.Theme,
	st *state.Store, h Hooks) {

	key := "boardview:" + node
	if !w.Claim(key) {
		// Already open. Recalled rather than ignored: a second press that
		// returns in silence is indistinguishable from a dead menu entry, and
		// for a window dragged out of reach the recall is the only way back.
		return
	}
	p := &Panel{Node: node, OnDo: h.OnDo, OnSaveShot: h.OnSaveShot}
	p.OnPopScreen = func(n string) { w.openScreen(n, p, newTheme, st) }
	go shell.RunPopout(w.WindowRegistry, key, "MeshBench - "+node+" board view",
		shell.PopoutSize{W: 1180, H: 720}, p, newTheme, st)
}

// openScreen puts the board's panel in a window of its own.
//
// It shares the ScreenView with the window it came from rather than making a
// second one, so the touch mapping, the key focus and the scale it was drawn at
// are one set of facts. Two copies would be two chances to divide by the wrong
// number, and dividing by the wrong number is silent.
func (w *WindowSet) openScreen(node string, from *Panel,
	newTheme func() *theme.Theme, st *state.Store) {

	key := "boardview-screen:" + node
	if !w.Claim(key) {
		return
	}
	sp := &ScreenPanel{Node: node, view: &from.screen, OnDo: from.OnDo}
	go shell.RunPopout(w.WindowRegistry, key, "MeshBench - "+node+" screen",
		shell.PopoutSize{W: 700, H: 560}, sp, newTheme, st)
}
