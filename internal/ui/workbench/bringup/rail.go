// The left column: the board's panel, how big it is drawn, and its parts as an index to pick from.
package bringup

import (
	"fmt"
	"image"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// rail is the board's panel with the parts under it.
func (p *Panel) rail(t *theme.Theme, gtx layout.Context, b hw.Board,
	st *state.NodeStat, rows []Row) layout.Dimensions {

	return onPanel(t, gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(t.Sp.S).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.panelHead(t, gtx, b)
				}),
				// Above the panel rather than under it. Under it, the note is
				// the first thing pushed off the bottom when the panel is
				// turned up - which is exactly when what it says, the scale
				// and whether the board is powered, is worth reading.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.panelNote(t, gtx, b, st)
				}),
				// Lamps above the panel and the things somebody can press below
				// it, which follows from the kind rather than from a position:
				// none of this is a photograph, and a schematic that reads
				// correctly is worth more than a likeness.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Bottom: t.Sp.XXS}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return p.parts.Lamps(t, gtx, b.Hardware)
						})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.screen.Layout(t, gtx, b, st, p.scale, p.OnDo, p.Node)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.controls(t, gtx, b)
				}),
				// The parts scroll. At 2:1 the panel takes most of the rail and
				// a list quietly cut off is a board that looks like it has
				// fewer parts than it has.
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return p.partsIndex(t, gtx, rows)
				}),
			)
		})
	})
}

// panelHead carries the scale steps and the way out.
//
// Steps rather than a zoom: the only honest sizes are whole multiples, so a
// continuous control would offer a hundred positions of which three are real.
func (p *Panel) panelHead(t *theme.Theme, gtx layout.Context, b hw.Board) layout.Dimensions {
	cur, _, _ := boxFor(b, p.scale)
	return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, "PANEL")),
			layout.Flexed(1, spacer),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				var kids []layout.FlexChild
				for i := range p.steps {
					n, col := i+1, t.P.Faint
					if n == cur {
						col = t.P.Accent
					}
					kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.steps[i].Layout(gtx,
							comp.Mono(t, t.Sz.Caption, col, fmt.Sprintf(" %d:1", n)))
					}))
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
			}),
			layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.popScreen.Layout(t, gtx)
			}),
		)
	})
}

// panelNote says how big the panel is and at what scale, and what it is doing
// when it is not drawing.
//
// The scale is on it because a panel at 1:1 and one at 2:1 are different
// evidence and nothing else on the screen would say which this is.
func (p *Panel) panelNote(t *theme.Theme, gtx layout.Context, b hw.Board,
	st *state.NodeStat) layout.Dimensions {

	sc := b.Hardware.Screen
	if sc == nil {
		return layout.Dimensions{}
	}
	scale, _, _ := boxFor(b, p.scale)
	note := fmt.Sprintf("%d × %d · %s · %d:1", sc.WidthPx, sc.HeightPx, sc.Controller, scale)
	switch {
	case st == nil || !st.Running:
		note += " · not powered"
	case st.Screen == nil:
		note += " · nothing drawn yet"
	case !st.Screen.On:
		// Not a fault: the firmware switches the panel off after an idle.
		note += " · asleep"
	}
	return layout.Inset{Bottom: t.Sp.XXS}.Layout(gtx,
		comp.OneLine(t, t.Sz.Caption, t.P.Faint, note, true))
}

// controls is what somebody can press on this board: its buttons and its ball.
//
// Under the panel, and under the note that says what the panel is doing, so the
// order down the rail is the order a person reads it: what the board shows,
// what that means, and what can be done to it.
func (p *Panel) controls(t *theme.Theme, gtx layout.Context, b hw.Board) layout.Dimensions {
	return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.parts.Buttons(t, gtx, b.Hardware)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: t.Sp.XXS}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return p.parts.Ball(t, gtx, b.Hardware)
						})
				}),
			)
		})
}

// partsIndex is the rows as a list to pick from, grouped.
func (p *Panel) partsIndex(t *theme.Theme, gtx layout.Context, rows []Row) layout.Dimensions {
	items := indexItems(rows)
	return p.index.Layout(gtx, len(items), func(gtx layout.Context, i int) layout.Dimensions {
		it := items[i]
		if it.head != "" {
			return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XXS}.Layout(gtx,
				comp.Text(t, t.Sz.Caption, t.P.Faint, upper(it.head)))
		}
		r := rows[it.row]
		ink := t.P.Dim
		if it.row == p.sel {
			ink = t.P.Ink
		}
		return layout.Inset{Top: t.Sp.XXS, Bottom: t.Sp.XXS}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				if it.row == p.sel {
					comp.RoundRect(gtx, image.Pt(gtx.Constraints.Max.X,
						gtx.Dp(unit.Dp(18))), 4, t.P.Selected)
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return dot(gtx, r.Verdict.Colour(t))
					}),
					layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout),
					layout.Rigid(comp.OneLine(t, t.Sz.Caption, ink, r.Name, true)),
					layout.Flexed(1, spacer),
					layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Faint, r.Where)),
				)
			})
	})
}

// indexItem is one line of the index: a heading, or a row by position.
type indexItem struct {
	head string
	row  int
}

// indexItems groups the rows in the order they appear, heading each run.
//
// By first appearance rather than a fixed list, because the two tables have
// different groups and a list here would have to be kept in step with both.
func indexItems(rows []Row) []indexItem {
	var out []indexItem
	last := ""
	for i, r := range rows {
		if r.Group != last {
			last = r.Group
			out = append(out, indexItem{head: r.Group})
		}
		out = append(out, indexItem{row: i})
	}
	return out
}
