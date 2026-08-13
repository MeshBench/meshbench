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
	"sort"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
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
	snap     *state.Snapshot
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
}

type menu struct {
	name  string
	click widget.Clickable
	items []MenuItem
	// x is where the menu bar actually laid this title out, filled during the
	// bar's own layout. The first attempt guessed it as 8 + 74*index, which is
	// right for the first menu and wrong for every other one, because menu
	// titles are different widths.
	x int
}

// MenuItem is one entry of a menu, named for what it does and carrying an
// action rather than a closure, so a click and a script take one route.
type MenuItem struct {
	Label    string
	Action   string
	Shortcut string
	click    widget.Clickable
}

// WindowMenu fills a menu with every panel that can become a window.
//
// Generated rather than hand-listed. A written list is a list that goes stale
// the moment a panel is added, and the symptom is somebody looking for a panel
// they had before and not finding it - which is exactly what a hand-written
// Window menu of three entries did.
func (sh *Shell) WindowMenu(name string) {
	names := make([]string, 0, len(sh.Panels))
	for n, p := range sh.Panels {
		if p != nil && p.Windowable {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	items := make([]MenuItem, 0, len(names))
	for _, n := range names {
		items = append(items, MenuItem{Label: n, Action: "panel." + n})
	}
	sh.SetMenu(name, items)
}

// SetMenu gives a menu its entries.
func (sh *Shell) SetMenu(name string, items []MenuItem) {
	for i := range sh.menus {
		if sh.menus[i].name == name {
			sh.menus[i].items = items
			return
		}
	}
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

func (sh *Shell) menuBar(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	// One row, said here rather than left to the children.
	//
	// A rigid child of a vertical flex is offered the whole remaining height,
	// and anything inside that fills it - a VRule, a flex with Middle
	// alignment - takes all of it and squeezes the body to nothing. Twice now.
	// Constraining the bar is the fix that holds however its contents change.
	gtx.Constraints.Max.Y = gtx.Dp(t.RowHeight())
	gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
	comp.Fill(gtx, t.P.Panel)
	children := make([]layout.FlexChild, 0, len(sh.menus)+2)
	// The running x, accumulated as each title is measured, so a dropdown
	// opens under the title it belongs to whatever the titles are called.
	runX := gtx.Dp(t.Sp.XS)
	for i := range sh.menus {
		m := &sh.menus[i]
		idx := i
		if m.click.Clicked(gtx) {
			if sh.openMenu == idx {
				sh.openMenu = -1
			} else {
				sh.openMenu = idx
			}
		}
		fg := t.P.Dim
		if sh.openMenu == idx {
			fg = t.P.Ink
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			d := m.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{
					Left: t.Sp.S, Right: t.Sp.S, Top: t.Sp.XS, Bottom: t.Sp.XS,
				}.Layout(gtx, comp.Text(t, t.Sz.Body, fg, m.name))
			})
			m.x = runX
			runX += d.Size.X
			return d
		}))
	}
	children = append(children, layout.Rigid(comp.VRule(t)))
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return sh.transportBar(t, gtx, sh.snap)
	}))
	children = append(children, layout.Flexed(1, comp.Spacer))
	// The honesty line, in the chrome, where it cannot be missed.
	children = append(children, layout.Rigid(
		comp.Text(t, t.Sz.Caption, t.P.Warn,
			"results are a best case: no multipath, bare earth, ideal demodulator")))
	return layout.Inset{Left: t.Sp.XS, Right: t.Sp.M, Top: t.Sp.XXS, Bottom: t.Sp.XXS}.
		Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		})
}

func (sh *Shell) viewBar(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	gtx.Constraints.Max.Y = gtx.Dp(t.RowHeight())
	gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
	comp.Fill(gtx, t.P.Panel)
	children := make([]layout.FlexChild, 0, numViews+3)
	for i := 0; i < int(numViews); i++ {
		i := i
		v := View(i)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			on := sh.View == v
			return sh.tabs[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				fg := t.P.Dim
				if on {
					fg = t.P.AccentInk
				}
				macro := layout.Inset{
					Left: t.Sp.M, Right: t.Sp.M, Top: t.Sp.S, Bottom: t.Sp.S,
				}
				dims := macro.Layout(gtx, comp.Text(t, t.Sz.Body, fg, v.String()))
				if on {
					comp.RoundRect(gtx, dims.Size, 6, t.P.Accent)
					macro.Layout(gtx, comp.Text(t, t.Sz.Body, fg, v.String()))
				}
				return dims
			})
		}))
	}
	children = append(children, layout.Flexed(1, comp.Spacer))
	children = append(children, layout.Rigid(
		comp.Mono(t, t.Sz.Caption, t.P.Dim, sh.counts(s))))
	return layout.Inset{Left: t.Sp.XS, Right: t.Sp.M}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		})
}

func (sh *Shell) counts(s *state.Snapshot) string {
	if s == nil {
		return ""
	}
	return itoa(len(s.Nodes)) + " nodes   seed " + itoa64(s.Seed) +
		"   t = " + msToS(s.NowMs)
}

func (sh *Shell) body(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	mainName, sideNames := layoutFor(sh.View)
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return sh.panel(t, gtx, s, mainName)
		}),
		layout.Rigid(comp.VRule(t)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			w := gtx.Dp(unit.Dp(340))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
			children := make([]layout.FlexChild, 0, len(sideNames)*2)
			for i, n := range sideNames {
				n := n
				if i > 0 {
					children = append(children, layout.Rigid(comp.HRule(t)))
				}
				children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return sh.panel(t, gtx, s, n)
				}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		}),
	)
}

// panel draws one panel with its chrome: a title, a pop-out affordance where
// the panel supports it, and the body.
func (sh *Shell) panel(t *theme.Theme, gtx layout.Context, s *state.Snapshot, name string) layout.Dimensions {
	p := sh.Panels[name]
	comp.Fill(gtx, t.P.Panel)
	return layout.Inset{
		Left: t.Sp.M, Right: t.Sp.M, Top: t.Sp.S, Bottom: t.Sp.S,
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(comp.SectionTitle(t, name)),
					layout.Flexed(1, comp.Spacer),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if p == nil || !p.Windowable {
							return layout.Dimensions{}
						}
						return sh.popOut[name].Layout(gtx,
							comp.Text(t, t.Sz.Caption, t.P.Accent, "open in its own window"))
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if p == nil {
					return layout.Center.Layout(gtx,
						comp.Text(t, t.Sz.Caption, t.P.Faint, "not built yet"))
				}
				if sh.PoppedOut != nil && sh.PoppedOut(name) {
					// Said, not left blank: a panel that has gone somewhere
					// looks identical to one that has broken.
					return layout.Center.Layout(gtx,
						comp.Text(t, t.Sz.Caption, t.P.Dim, "in its own window"))
				}
				return p.Draw(t, gtx, s)
			}),
		)
	})
}

func (sh *Shell) statusBar(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	comp.Fill(gtx, t.P.Panel)
	msg := sh.View.Purpose()
	if s != nil && s.Status != "" {
		msg = s.Status
	}
	return layout.Inset{Left: t.Sp.M, Right: t.Sp.M, Top: t.Sp.XS, Bottom: t.Sp.XS}.
		Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, msg)),
				layout.Flexed(1, comp.Spacer),
				layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Faint, "Gio")),
			)
		})
}

// EmptyPanel is a placeholder that says what will be here, so an unbuilt view
// reads as unfinished rather than broken.
func EmptyPanel(name, what string) *Panel {
	return &Panel{
		Name:       name,
		Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, _ *state.Snapshot) layout.Dimensions {
			return layout.Center.Layout(gtx,
				comp.Text(t, t.Sz.Caption, t.P.Faint, what))
		},
	}
}

// menuDrop draws the open menu's entries over the frame.
//
// At the top level rather than inside the bar: a dropdown clipped to a
// one-row-tall bar is a dropdown nobody can read. Its x comes from where the
// bar actually laid the title out.
func (sh *Shell) menuDrop(t *theme.Theme, gtx layout.Context) {
	if sh.openMenu < 0 || sh.openMenu >= len(sh.menus) {
		return
	}
	m := &sh.menus[sh.openMenu]
	if len(m.items) == 0 {
		return
	}
	for i := range m.items {
		if m.items[i].click.Clicked(gtx) {
			sh.openMenu = -1
			if sh.OnMenu != nil {
				sh.OnMenu(m.items[i].Action)
			}
			return
		}
	}
	pad := gtx.Dp(t.Sp.S)
	inner := gtx
	inner.Constraints.Min = image.Point{}
	inner.Constraints.Max = image.Pt(gtx.Dp(340), gtx.Constraints.Max.Y)

	rec := op.Record(gtx.Ops)
	var kids []layout.FlexChild
	for i := range m.items {
		i := i
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return m.items[i].click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if m.items[i].click.Hovered() {
					comp.FillRect(gtx, image.Pt(gtx.Constraints.Max.X,
						gtx.Dp(t.RowHeight())), theme.Alpha(t.P.Accent, 0.18))
				}
				return layout.Inset{Left: t.Sp.S, Right: t.Sp.S,
					Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{}.Layout(gtx,
							layout.Rigid(comp.Text(t, t.Sz.Body, t.P.Ink, m.items[i].Label)),
							layout.Flexed(1, comp.Spacer),
							layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Faint,
								m.items[i].Shortcut)),
						)
					})
			})
		}))
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(inner, kids...)
	content := rec.Stop()

	box := image.Pt(dims.Size.X+pad*2, dims.Size.Y+pad*2)
	x := m.x
	if x+box.X > gtx.Constraints.Max.X {
		x = gtx.Constraints.Max.X - box.X
	}
	off := op.Offset(image.Pt(x, gtx.Dp(t.RowHeight()))).Push(gtx.Ops)
	defer off.Pop()
	paint.FillShape(gtx.Ops, theme.Alpha(t.P.Panel, 0.98), clip.Rect{Max: box}.Op())
	comp.Border(gtx, box, 2, 1, t.P.Rule)
	in := op.Offset(image.Pt(pad, pad)).Push(gtx.Ops)
	content.Add(gtx.Ops)
	in.Pop()
}

// MenuX is where a menu's title sits in the bar, or -1 if there is no such
// menu. For a test that has to press the thing where it actually is rather
// than where a coordinate written down in advance says it should be.
func (sh *Shell) MenuX(name string) int {
	for i := range sh.menus {
		if sh.menus[i].name == name {
			return sh.menus[i].x
		}
	}
	return -1
}

// MenuItems is what a named menu carries, for the same reason.
func (sh *Shell) MenuItems(name string) []MenuItem {
	for i := range sh.menus {
		if sh.menus[i].name == name {
			return sh.menus[i].items
		}
	}
	return nil
}

// OpenMenuIndex is which menu is open, or -1. A menu that will not open is a
// different fault from one whose entries do nothing.
func (sh *Shell) OpenMenuIndex() int { return sh.openMenu }
