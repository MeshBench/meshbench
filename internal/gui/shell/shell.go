// Package shell is the application frame: the window, the view switcher, the
// menu bar, the status bar, and the panel arrangement for each view.
//
// A view is a declared layout, not an accumulated dock state. That is the
// deliberate departure from the old design, where every panel was a dock and
// the arrangement became a thing to be managed rather than chosen. Panels that
// genuinely want to move become real windows instead.
package shell

import (
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// Panel is one region of a view.
type Panel struct {
	Name string
	// Draw renders the panel from a snapshot. Panels never see mutable state.
	Draw func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions
	// Windowable panels can leave the layout and become an OS window. Every
	// panel in the old design could, and scripts rely on panel.pop_out being
	// generic, so this defaults to true.
	Windowable bool
	// InWindowMenu lists the panel in the Window menu itself. The menu had
	// become a dumping ground of every panel alphabetically; the daily set
	// is listed, and the rest stay one step away behind "Show all panels...".
	InWindowMenu bool
}

// Shell is the whole frame.
type Shell struct {
	View     View
	Panels   map[string]*Panel
	tabs     [numViews]widget.Clickable
	menus    []menu
	Status   string
	OnPopOut func(name string)
	// OnMenu is called with a menu entry's action.
	OnMenu func(action string)
	// openMenu is which menu is showing, or -1. Not zero: zero is the first
	// menu, and a zero value that means "File is open" opens it on launch.
	openMenu int
	tr       transport
	// rfChip is the counts line's physics toggle.
	rfChip widget.Clickable
	snap   *state.Snapshot
	// PoppedOut reports whether a panel is currently living in its own window.
	//
	// A panel that has moved out must not also be drawn here. Two frame loops
	// laying out one panel share its widget state - one list's scroll
	// position, one editor's caret, one table's macros - and Gio's ops are not
	// safe for that: the first attempt crashed with an unfinished child, which
	// is one goroutine's macro being closed by another's.
	PoppedOut func(name string) bool
	popOut    map[string]*widget.Clickable
	// Ask is the one question the shell can put on screen. A menu entry
	// carries no parameters, so a verb needing a name had nowhere to get one.
	Ask Prompt

	// sizes is what the operator has dragged a panel to, in dp, keyed by what
	// the splitter sizes. Absent means the arrangement's own figure, so a view
	// nobody has resized behaves exactly as declared.
	sizes map[string]int
	// splitters are pooled by the same key: a widget rebuilt each frame is a
	// widget that never sees the release that ends a drag.
	splitters map[string]*comp.Splitter
	// lastRow is how tall each row came out last frame, in dp.
	lastRow map[string]int
}

// New builds the shell with the standard menus.
func New() *Shell {
	sh := &Shell{
		Panels:   map[string]*Panel{},
		popOut:   map[string]*widget.Clickable{},
		openMenu: -1,
	}
	for _, m := range []string{"File", "View", "Simulation", "Repeaters", "Planning", "Window", "Help"} {
		sh.menus = append(sh.menus, menu{name: m})
	}
	return sh
}

// Add registers a panel.
func (sh *Shell) Add(p *Panel) {
	sh.Panels[p.Name] = p
	sh.popOut[p.Name] = &widget.Clickable{}
}

// Layout draws the whole frame for one frame, from one snapshot.
func (sh *Shell) Layout(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	// Held for the frame so the menu bar's transport can read it without
	// threading a snapshot through every chrome function.
	sh.snap = s
	comp.Fill(gtx, t.P.Ground)
	sh.shortcuts(gtx)
	for i := range sh.tabs {
		if sh.tabs[i].Clicked(gtx) {
			sh.View = View(i)
		}
	}
	for name, c := range sh.popOut {
		if c.Clicked(gtx) && sh.OnPopOut != nil {
			sh.OnPopOut(name)
		}
	}
	// The dropdown draws over the frame, so it goes last - and the question
	// after it, because a question is modal and the dropdown is not.
	defer func() {
		sh.menuDrop(t, gtx)
		sh.Ask.Layout(t, gtx)
	}()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return sh.menuBar(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return sh.viewBar(t, gtx, s) }),
		layout.Rigid(comp.HRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return sh.body(t, gtx, s) }),
		layout.Rigid(comp.HRule(t)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return sh.statusBar(t, gtx, s) }),
	)
}
