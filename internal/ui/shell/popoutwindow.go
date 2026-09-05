// The event loop every pop-out window runs.
//
// One copy, at the third window that wanted it. A node window, a firmware
// window and an output window differ in what they draw, what they are called
// and how big they open; the loop around all three - the layer-shell chrome, a
// raise that is a wish rather than a reach into another goroutine, the frame,
// the theme with a shaper of its own - was the same forty lines each time.
//
// The bar is the loop's too, and that is the second lesson. It used to belong
// to the panel: three accessors to implement and a block to remember at the top
// of its own flex. Two of the five panels implemented the accessors and never
// laid the bar out - so those windows could not be dragged, maximised or
// closed, and because a layer surface is not resized by the compositor either,
// they were stuck at whatever size they opened at. Nothing failed. The panels
// that got it right made the ones that did not look like a platform quirk.
//
// So a panel says what its bar is called and draws its contents, and everything
// about being a window happens here, once, where it cannot be forgotten.
package shell

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

// PopoutPanel is what a pop-out window draws.
//
// Its contents and nothing else. Being a window - the bar, the drag, the
// maximise, the size that has to fit the screen - is the loop's business, and a
// panel that had to take part in it was a panel that could get it wrong.
type PopoutPanel interface {
	Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions
}

// DrawFunc is a PopoutPanel that is only a function, for a panel whose Draw is
// a field rather than a method - which the workbench's own panels are, being
// values in a table rather than types.
type DrawFunc func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions

func (f DrawFunc) Draw(t *theme.Theme, gtx layout.Context,
	s *state.Snapshot) layout.Dimensions {
	return f(t, gtx, s)
}

// Popout is one window: what it is known by, what it is called, and how big it
// would like to open.
type Popout struct {
	// Key is what the window is known by in the set. A second request for the
	// same key raises the one already out there rather than opening another
	// copy, and the caller has already claimed it.
	Key string
	// Title is what the desktop calls the window, where the desktop names
	// windows at all.
	Title string
	// Bar is what the window's own title bar says, which is shorter: the
	// desktop's title carries the application's name and the bar is inside it,
	// so repeating "MeshBench - " there wastes the width it has.
	Bar string
	// W and H are the size it would like. It may get less - see fitted.
	W, H unit.Dp

	// Under is drawn along the bottom, inside the window and below the panel.
	// The panel windows put the status line there, so an action started in one
	// says what happened in the same window rather than behind it.
	Under func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions
	// Over is drawn on top of everything, which is where a question belongs:
	// a popped-out panel asks in its own window, not in the main one.
	Over func(t *theme.Theme, gtx layout.Context)
	// Closing is asked once a frame, for a window something else decided
	// should go - the panel being put back in the main window, for one.
	Closing func() bool
}

// RunPopout opens one window and runs it until it closes.
func RunPopout(w *WindowRegistry, spec Popout, p PopoutPanel,
	newTheme func() *theme.Theme, st *state.Store) {

	defer w.Release(spec.Key)
	// A theme with a shaper of its own. Gio's is not safe for concurrent use,
	// and two frame loops sharing one corrupts its glyph buffer.
	th := newTheme()
	spot := float.NextSpot()
	win := new(app.Window)
	// Whether it stays above the main window is the machine's preference,
	// read once here because the ask only exists at creation.
	win.Option(append([]app.Option{
		app.Title(spec.Title),
		app.Size(spec.W, spec.H),
	}, float.Above(spot, KeepAbove(st))...)...)
	// Raised as it opens, for the platforms where above is not or cannot be
	// honoured. Where it was, the window is on the overlay layer and raising
	// is meaningless anyway.
	win.Perform(system.ActionRaise)

	var chrome *LayerChrome
	var bar comp.TitleBar
	var ops op.Ops
	sized := false
	for {
		switch e := win.Event().(type) {
		case app.ConfigEvent:
			if e.Config.LayerShell && chrome == nil {
				chrome = NewLayerChrome(spot)
			}
			if chrome != nil {
				chrome.Screens(e.Config.Output, e.Config.Outputs)
				// Once, as soon as an output is known: a window bigger than
				// the screen it landed on has no way to become smaller.
				if !sized {
					if opts := fitted(spec, chrome); len(opts) > 0 {
						sized = true
						win.Option(opts...)
					}
				}
			}
		case app.DestroyEvent:
			return
		case app.FrameEvent:
			if spec.Closing != nil && spec.Closing() {
				win.Perform(system.ActionClose)
			}
			if w.WantsRaise(spec.Key) {
				// Raising means nothing to a layer surface, so for a layered
				// window the wish recalls it on screen instead - which is also
				// how one dragged out of reach comes back.
				if chrome != nil {
					if opts := chrome.Recall(float.NextSpot()); len(opts) > 0 {
						win.Option(opts...)
					}
				} else {
					win.Perform(system.ActionRaise)
				}
			}
			gtx := app.NewContext(&ops, e)
			comp.Fill(gtx, th.P.Ground)
			if chrome != nil {
				chrome.Frame(e)
			}
			snap := st.Snapshot()
			layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if chrome == nil {
						return layout.Dimensions{}
					}
					bar.Title, bar.Maximised = spec.Bar, chrome.Maximised()
					return bar.Layout(th, gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(th.Sp.M).Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return p.Draw(th, gtx, snap)
						})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if spec.Under == nil {
						return layout.Dimensions{}
					}
					return spec.Under(th, gtx, snap)
				}),
			)
			if chrome != nil {
				opts, closing := chrome.Update(&bar)
				if closing {
					win.Perform(system.ActionClose)
				} else if len(opts) > 0 {
					win.Option(opts...)
				}
			}
			// Over everything in the window, which is where a question
			// belongs: a popped-out panel asks in its own window.
			if spec.Over != nil {
				spec.Over(th, gtx)
			}
			e.Frame(gtx.Ops)
			win.Invalidate()
		}
	}
}

// fitted is the options that make a window fit the screen it landed on, or
// nothing where it already does.
//
// A layer-shell surface is not resized by the compositor and has no
// decorations to drag, so a window that opens taller than the output stays that
// way: whatever runs off the bottom is unreachable for the life of the window.
// The board view found this the moment it grew to hold four regions, and the
// answer belongs here rather than in a size every caller has to guess
// conservatively - a window should ask for the room it wants and be given what
// there is.
func fitted(spec Popout, c *LayerChrome) []app.Option {
	screen := c.Screen()
	if screen.Empty() {
		return nil
	}
	// Room left at the edges so the window reads as a window rather than as
	// the desktop, and so a bar remains reachable at the bottom.
	const edge = unit.Dp(64)
	w, h := spec.W, spec.H
	if max := unit.Dp(screen.Dx()) - edge; w > max {
		w = max
	}
	if max := unit.Dp(screen.Dy()) - edge; h > max {
		h = max
	}
	opts := c.FitSpot(w, h)
	if w != spec.W || h != spec.H {
		opts = append(opts, app.Size(w, h))
	}
	return opts
}
