// Package shell is the application frame: the window, the view switcher, the
// menu bar, the status bar, and the panel arrangement for each view.
//
// A view is a declared layout, not an accumulated dock state. That is the
// deliberate departure from the old design, where every panel was a dock and
// the arrangement became a thing to be managed rather than chosen. Panels that
// genuinely want to move become real windows instead.
package shell

import (
	"image"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
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
	// Menu is where this panel is offered, by menu name, and Section groups
	// it within that menu. Every panel names one, because a panel nobody can
	// find from a menu is a panel that does not exist: the old arrangement
	// listed a curated thirteen and left twenty reachable only by a chooser
	// that then threw them out of the window.
	Menu    string
	Section string
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

	// layouts is each view's live arrangement, seeded from its preset on
	// first use. Nil means "not touched yet", which is what makes a view
	// nobody has visited behave exactly as declared.
	layouts [numViews]*Arrangement
	// focus is the region a panel opened from a menu lands in: the last one
	// pressed, so opening a panel puts it where the work is.
	focus regionRef
	// tabClicks are the tab strip's buttons and tabTags its tabs, both pooled
	// by region and name.
	tabClicks map[string]*widget.Clickable
	tabTags   map[string]*tabHandle
	// drag is the tab being carried, if any, and regionSize/regionRect are
	// what the drop test needs: Gio tells a widget its size and never its
	// position, so the positions are accumulated from the sizes.
	drag       *tabDrag
	regionSize map[regionRef]image.Point
	regionRect map[regionRef]image.Rectangle
	// bodyTop is how far down the window the panels begin - the menu bar,
	// the view bar and the rule between them. Recorded as they draw so the
	// region rectangles can be in the window's own coordinates, which is
	// what a pointer event speaks.
	bodyTop int

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
		focus:    noRegion,
	}
	// Repeaters widened to Mesh, which is what it always held - companions
	// and room servers are not repeaters - and Analysis is new: the study
	// panels had no home of their own and were the bulk of what could only
	// be reached by the chooser.
	for _, m := range MenuNames {
		sh.menus = append(sh.menus, menu{name: m})
	}
	return sh
}

// MenuNames are the menu bar's menus, in order. Exported because the
// workbench's own table has to name the same ones: SetMenu silently ignores a
// name that is not here, so a typo would be a menu that never appears.
var MenuNames = []string{"File", "View", "Simulation", "Mesh", "Analysis", "Window", "Help"}

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
	top := 0
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			d := sh.menuBar(t, gtx)
			top += d.Size.Y
			return d
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			d := sh.viewBar(t, gtx, s)
			top += d.Size.Y
			return d
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			d := comp.HRule(t)(gtx)
			top += d.Size.Y
			sh.bodyTop = top
			return d
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return sh.body(t, gtx, s) }),
		layout.Rigid(comp.HRule(t)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return sh.statusBar(t, gtx, s) }),
	)
}
