// Panels that leave the layout and become real windows (P7).
//
// This is the departure from the old design that started the whole exercise. A
// dock is the right answer when two things are read together; it is the wrong
// answer when one of them wants to be on the other monitor, and imgui's docking
// made everything a dock because that was the only thing it had. Here a panel
// that wants to be a window is a window - a real one, with its own frame loop,
// its own size, and the compositor's own decorations.
package main

import (
	"fmt"
	"sync"

	"gioui.org/io/system"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/shell"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// windows tracks what has been popped out, so a second click on the same
// affordance raises the window rather than opening another copy of it.
type windows struct {
	mu   sync.Mutex
	open map[string]bool
	// closing is which windows have been asked to go away, read by each
	// window's own loop.
	closing map[string]bool
}

func newWindows() *windows { return &windows{open: map[string]bool{}} }

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
		w.mu.Unlock()
		return
	}
	w.open[name] = true
	w.mu.Unlock()

	go func() {
		defer func() {
			w.mu.Lock()
			delete(w.open, name)
			w.mu.Unlock()
		}()
		th := newTheme()
		win := new(app.Window)
		// Titled for the panel, so a taskbar with six of these open is
		// readable. The app id stays the same: these are windows of one
		// application, not six applications.
		win.Option(app.Title("MeshBench - "+name),
			app.Size(unit.Dp(900), unit.Dp(640)))
		var ops op.Ops
		for {
			switch e := win.Event().(type) {
			case app.DestroyEvent:
				return
			case app.FrameEvent:
				if w.wantsClose(name) {
					win.Perform(system.ActionClose)
				}
				gtx := app.NewContext(&ops, e)
				comp.Fill(gtx, th.P.Ground)
				layout.UniformInset(th.Sp.M).Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return p.Draw(th, gtx, st.Snapshot())
					})
				e.Frame(gtx.Ops)
				win.Invalidate()
			}
		}
	}()
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
