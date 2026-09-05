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
	"sync"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// windows tracks what has been popped out, so a second click on the same
// affordance raises the window rather than opening another copy of it.
type panelPopouts struct {
	// reg is the same bookkeeping every other pop-out window uses: which are
	// open, and the wishes to raise or close one from another goroutine. This
	// set kept its own copy of it, and its own copy of the frame loop with it,
	// which is how the two drifted.
	reg *shell.WindowRegistry

	mu sync.Mutex
	// prompts is each popped-out window's own question overlay. A question
	// asked from a panel belongs in the window the panel is in - the shared
	// prompt lives in the main window, and a dialog appearing there while the
	// person is working in a pop-out is a dialog nobody finds.
	//
	// The one thing here the registry has no opinion about, so the one thing
	// still kept beside it.
	prompts map[string]*shell.Prompt
}

func newPanelPopouts() *panelPopouts {
	return &panelPopouts{reg: shell.NewWindowRegistry(),
		prompts: map[string]*shell.Prompt{}}
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
func (w *panelPopouts) popOut(name string, sh *shell.Shell, newTheme func() *theme.Theme,
	st *state.Store) {
	p := sh.Panels[name]
	if p == nil || !p.Windowable {
		return
	}
	if !w.reg.Claim(name) {
		// Already out there, and Claim has asked it forward. A second press
		// used to return in silence, which from the far side of the screen is
		// indistinguishable from a dead menu entry.
		return
	}
	ask := &shell.Prompt{}
	w.mu.Lock()
	w.prompts[name] = ask
	w.mu.Unlock()

	go func() {
		defer func() {
			w.mu.Lock()
			delete(w.prompts, name)
			w.mu.Unlock()
		}()
		shell.RunPopout(w.reg, shell.Popout{
			Key: name, Title: "MeshBench - " + name, Bar: name, W: 900, H: 640,
			// The status line, exactly as the main window shows it. An action
			// started here says what happened here - "firmware failed, so the
			// run has not started" used to appear only in the main window,
			// behind this one.
			Under: popStatus,
			// This window's own questions, over everything in it.
			Over:    func(t *theme.Theme, gtx layout.Context) { ask.Layout(t, gtx) },
			Closing: func() bool { return w.reg.WantsClose(name) },
		}, shell.DrawFunc(p.Draw), newTheme, st)
	}()
}

// promptFor is where a question from the named panel belongs right now: the
// panel's own window if it is popped out, otherwise the main window's prompt.
//
// The panel name is enough to decide because a panel is drawn in one window
// at a time - popping out is what removes it from the main layout.
func (w *panelPopouts) promptFor(name string, main *shell.Prompt) *shell.Prompt {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p, ok := w.prompts[name]; ok && w.reg.Has(name) {
		return p
	}
	return main
}

// has reports whether a panel is currently popped out.
func (w *panelPopouts) has(name string) bool { return w.reg.Has(name) }

// dock asks a popped-out window to go away.
//
// The window owns its own event loop, so this cannot close it directly: it
// records the wish and the loop reads it on its next frame. Closing a window
// from another goroutine is how Gio's event queue ends up with a destroyed
// window still in it.
func (w *panelPopouts) dock(name string) { w.reg.AskClose(name) }

// raise asks a window to come to the front on its next frame.
func (w *panelPopouts) raise(name string) {
	// Claim is the ask: it returns false for a window already out there and
	// records the wish as it does, which is the same path a second press takes.
	if w.reg.Claim(name) {
		// It was not open after all, so nothing was raised and nothing should
		// be left claimed.
		w.reg.Release(name)
	}
}

// names is every panel currently in a window of its own.
func (w *panelPopouts) names() []string { return w.reg.Keys() }

func (w *panelPopouts) close(name string) error {
	if !w.reg.Has(name) {
		return fmt.Errorf("%s is not in its own window", name)
	}
	w.dock(name)
	return nil
}

// popStatus is the one-line footer a popped-out window shares with the main
// one: the status string, and whichever job is still running - so a long
// operation announces itself wherever the person actually is.
func popStatus(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	comp.Fill(gtx, t.P.Panel)
	msg := ""
	if s != nil {
		msg = s.Status
		// The same line the main window shows, from the same function: two
		// copies of this had already drifted into showing different jobs.
		if line := shell.JobWords(s.Jobs); line != "" {
			msg = line
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
