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
	// PoppedOut reports whether a panel is currently living in its own window.
	//
	// A panel that has moved out must not also be drawn here. Two frame loops
	// laying out one panel share its widget state - one list's scroll
	// position, one editor's caret, one table's macros - and Gio's ops are not
	// safe for that: the first attempt crashed with an unfinished child, which
	// is one goroutine's macro being closed by another's.
	PoppedOut func(name string) bool
	popOut    map[string]*widget.Clickable
}

type menu struct {
	name  string
	click widget.Clickable
}

// New builds the shell with the standard menus.
func New() *Shell {
	sh := &Shell{
		Panels: map[string]*Panel{},
		popOut: map[string]*widget.Clickable{},
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
	comp.Fill(gtx, t.P.Panel)
	children := make([]layout.FlexChild, 0, len(sh.menus)+2)
	for i := range sh.menus {
		m := &sh.menus[i]
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return m.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{
					Left: t.Sp.S, Right: t.Sp.S, Top: t.Sp.XS, Bottom: t.Sp.XS,
				}.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim, m.name))
			})
		}))
	}
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
