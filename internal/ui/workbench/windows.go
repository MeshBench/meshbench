// Panels that leave the layout and become real windows.
//
// This is the departure from the old design that started the whole exercise. A
// dock is the right answer when two things are read together; it is the wrong
// answer when one of them wants to be on the other monitor, and imgui's docking
// made everything a dock because that was the only thing it had. Here a panel
// that wants to be a window is a window - a real one, with its own frame loop,
// its own size, and the compositor's own decorations.
package workbench

import (
	"fmt"
	"sort"
	"sync"

	"gioui.org/io/system"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/float"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// windows tracks what has been popped out, so a second click on the same
// affordance raises the window rather than opening another copy of it.
type windows struct {
	mu   sync.Mutex
	open map[string]bool
	// closing is which windows have been asked to go away, and raising which
	// have been asked to come forward. Both are wishes rather than actions:
	// a window belongs to its own event loop, and performing anything on it
	// from another goroutine is how a destroyed window stays in Gio's queue.
	closing map[string]bool
	raising map[string]bool
	// prompts is each popped-out window's own question overlay. A question
	// asked from a panel belongs in the window the panel is in - the shared
	// prompt lives in the main window, and a dialog appearing there while the
	// person is working in a pop-out is a dialog nobody finds.
	prompts map[string]*shell.Prompt
}

func newWindows() *windows {
	return &windows{open: map[string]bool{}, prompts: map[string]*shell.Prompt{}}
}

// popOut gives a panel its own window.
//
// Each window runs its own event loop on its own goroutine and reads the same
// snapshot as the main one. That is the whole benefit of the state layer: two
// windows drawing one world need no coordination beyond both reading an
// immutable value.
// newTheme must return a theme with a shaper of its own. Gio's text shaper is
// not safe for concurrent use, and two frame loops sharing one corrupts its
// glyph buffer: the first pop-out crashed on
// text.Shaper.NextGlyph with an index out of range, from an editor in another
// window drawing at the same moment. One shaper per window is the fix, and it
// costs a font cache rather than correctness.
func (w *windows) popOut(name string, sh *shell.Shell, newTheme func() *theme.Theme,
	st *state.Store) {
	p := sh.Panels[name]
	if p == nil || !p.Windowable {
		return
	}
	w.mu.Lock()
	if w.open[name] {
		// Already out there: raise it rather than doing nothing. A second
		// press used to return in silence, which from the far side of the
		// screen is indistinguishable from a dead menu entry - and on Linux
		// the window it was asking for had usually fallen behind this one.
		w.mu.Unlock()
		w.raise(name)
		return
	}
	w.open[name] = true
	ask := &shell.Prompt{}
	w.prompts[name] = ask
	w.mu.Unlock()

	go func() {
		defer func() {
			w.mu.Lock()
			delete(w.open, name)
			delete(w.prompts, name)
			w.mu.Unlock()
		}()
		th := newTheme()
		spot := float.NextSpot()
		win := new(app.Window)
		// Titled for the panel, so a taskbar with six of these open is
		// readable. The app id stays the same: these are windows of one
		// application, not six applications. Whether it stays above the
		// others is the machine's preference, read once here because the
		// ask only exists at creation.
		win.Option(append([]app.Option{
			app.Title("MeshBench - " + name),
			app.Size(unit.Dp(900), unit.Dp(640)),
		}, float.Above(spot, keepAbove(st))...)...)
		// Raised as it opens, for the platforms where above is not or
		// cannot be honoured - the preference off, GNOME, X11 - so the
		// window at least starts in front rather than behind.
		win.Perform(system.ActionRaise)
		// A layer-shell surface carries no decoration of the compositor's,
		// so this window draws its own title bar when the ask was granted;
		// the chrome is the machinery under it.
		var layered bool
		var bar comp.TitleBar
		var chrome *layerChrome
		var ops op.Ops
		for {
			switch e := win.Event().(type) {
			case app.ConfigEvent:
				if e.Config.LayerShell && !layered {
					layered, chrome = true, newLayerChrome(spot)
					bar.Title = "MeshBench - " + name
				}
				if layered {
					chrome.outputSize(e.Config.OutputSize)
				}
			case app.DestroyEvent:
				return
			case app.FrameEvent:
				if w.wantsClose(name) {
					win.Perform(system.ActionClose)
				}
				if w.wantsRaise(name) {
					// Raising means nothing to a layer surface, so for a
					// layered window the wish recalls it on screen instead -
					// which is also how one dragged out of reach comes back.
					if layered {
						if opts := chrome.recall(float.NextSpot()); len(opts) > 0 {
							win.Option(opts...)
						}
					} else {
						win.Perform(system.ActionRaise)
					}
				}
				gtx := app.NewContext(&ops, e)
				comp.Fill(gtx, th.P.Ground)
				snap := st.Snapshot()
				var kids []layout.FlexChild
				if layered {
					chrome.frame(e)
					kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return bar.Layout(th, gtx)
					}))
				}
				kids = append(kids,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.UniformInset(th.Sp.M).Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								return p.Draw(th, gtx, snap)
							})
					}),
					// The status line, exactly as the main window shows it.
					// An action started here says what happened here -
					// "firmware failed, so the run has not started" used to
					// appear only in the main window, behind this one.
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return popStatus(th, gtx, snap)
					}),
				)
				layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
				if layered {
					if opts, close := chrome.update(&bar); close {
						win.Perform(system.ActionClose)
					} else if len(opts) > 0 {
						win.Option(opts...)
					}
				}
				// This window's own questions, over everything in it.
				ask.Layout(th, gtx)
				e.Frame(gtx.Ops)
				win.Invalidate()
			}
		}
	}()
}

// promptFor is where a question from the named panel belongs right now: the
// panel's own window if it is popped out, otherwise the main window's prompt.
//
// The panel name is enough to decide because a panel is drawn in one window
// at a time - popping out is what removes it from the main layout.
func (w *windows) promptFor(name string, main *shell.Prompt) *shell.Prompt {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p, ok := w.prompts[name]; ok && w.open[name] {
		return p
	}
	return main
}

// has reports whether a panel is currently popped out.
func (w *windows) has(name string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.open[name]
}

// dock and close ask a popped-out window to go away.
//
// The window owns its own event loop, so this cannot close it directly: it
// records the wish and the loop reads it on its next frame. Closing a window
// from another goroutine is how Gio's event queue ends up with a destroyed
// window still in it.
func (w *windows) dock(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closing == nil {
		w.closing = map[string]bool{}
	}
	if w.open[name] {
		w.closing[name] = true
	}
}

// raise asks a window to come to the front on its next frame.
func (w *windows) raise(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.raising == nil {
		w.raising = map[string]bool{}
	}
	if w.open[name] {
		w.raising[name] = true
	}
}

// wantsRaise reports and clears the wish, so one ask is one raise.
func (w *windows) wantsRaise(name string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.raising[name] {
		delete(w.raising, name)
		return true
	}
	return false
}

// names is every panel currently in a window of its own.
func (w *windows) names() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, 0, len(w.open))
	for n := range w.open {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (w *windows) close(name string) error {
	w.mu.Lock()
	open := w.open[name]
	w.mu.Unlock()
	if !open {
		return fmt.Errorf("%s is not in its own window", name)
	}
	w.dock(name)
	return nil
}

// wantsClose reports and clears the wish, on the window's own goroutine.
func (w *windows) wantsClose(name string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closing[name] {
		delete(w.closing, name)
		return true
	}
	return false
}

// popStatus is the one-line footer a popped-out window shares with the main
// one: the status string, and whichever job is still running - so a long
// operation announces itself wherever the person actually is.
func popStatus(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	comp.Fill(gtx, t.P.Panel)
	msg := ""
	if s != nil {
		msg = s.Status
		for i := range s.Jobs {
			if !s.Jobs[i].Finished {
				j := &s.Jobs[i]
				if j.Total > 0 {
					msg = fmt.Sprintf("%s - %d%% (%d of %d)",
						j.What, j.Done*100/j.Total, j.Done, j.Total)
				} else {
					msg = j.What
				}
				break
			}
		}
	}
	return layout.Inset{Left: t.Sp.M, Right: t.Sp.M, Top: t.Sp.XS, Bottom: t.Sp.XS}.
		Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, msg)),
				layout.Flexed(1, comp.Spacer),
			)
		})
}
