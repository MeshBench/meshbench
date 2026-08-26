// The event loop every pop-out window runs.
//
// One copy, at the third window that wanted it. A node window, a firmware
// window and an output window differ in what they draw, what they are called
// and how big they open; the loop around all three - the layer-shell chrome, a
// raise that is a wish rather than a reach into another goroutine, the frame,
// the theme with a shaper of its own - was the same forty lines each time.
package workbench

import (
	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/float"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// popoutPanel is what a pop-out window draws, and the little of its own state
// the loop has to reach.
//
// The three accessors exist because a compositor that draws no decorations
// leaves the window to draw its own, and the bar belongs to the panel: it is
// the panel that knows the title and the panel that lays it out at the top of
// its own flex.
type popoutPanel interface {
	Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions
	// setLayered tells the panel it is a layer-shell surface and must draw
	// its own title bar.
	setLayered(bool)
	// titleBar is that bar, so the chrome can read what was pressed on it.
	titleBar() *comp.TitleBar
	// setMaximised carries the chrome's restore state back, so the bar draws
	// the right glyph.
	setMaximised(bool)
}

// popoutSize is how big a window opens.
type popoutSize struct{ w, h unit.Dp }

// runPopout opens one window and runs it until it closes.
//
// key is what the window is known by in the set: a second request for the same
// key raises the window already out there rather than opening another copy.
// The caller has already claimed it.
func runPopout(w *windowSet, key, title string, size popoutSize,
	p popoutPanel, newTheme func() *theme.Theme, st *state.Store) {

	defer w.release(key)
	// A theme with a shaper of its own. Gio's is not safe for concurrent use,
	// and two frame loops sharing one corrupts its glyph buffer.
	th := newTheme()
	spot := float.NextSpot()
	win := new(app.Window)
	// Whether it stays above the main window is the machine's preference,
	// read once here because the ask only exists at creation.
	win.Option(append([]app.Option{
		app.Title(title),
		app.Size(size.w, size.h),
	}, float.Above(spot, keepAbove(st))...)...)
	// Raised as it opens, for the platforms where above is not or cannot be
	// honoured. Where it was, the window is on the overlay layer and raising
	// is meaningless anyway.
	win.Perform(system.ActionRaise)

	var chrome *layerChrome
	var ops op.Ops
	for {
		switch e := win.Event().(type) {
		case app.ConfigEvent:
			if e.Config.LayerShell && chrome == nil {
				chrome = newLayerChrome(spot)
				p.setLayered(true)
			}
			if chrome != nil {
				chrome.screens(e.Config.Output, e.Config.Outputs)
			}
		case app.DestroyEvent:
			return
		case app.FrameEvent:
			if w.wantsRaise(key) {
				// Raising means nothing to a layer surface, so for a layered
				// window the wish recalls it on screen instead - which is also
				// how one dragged out of reach comes back.
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
				opts, closing := chrome.update(p.titleBar())
				p.setMaximised(chrome.maximised)
				if closing {
					win.Perform(system.ActionClose)
				} else if len(opts) > 0 {
					win.Option(opts...)
				}
			}
			e.Frame(gtx.Ops)
			win.Invalidate()
		}
	}
}
