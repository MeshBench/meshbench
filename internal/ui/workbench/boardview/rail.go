// The left column: the board's panel, how big it is drawn, and its parts as an index to pick from.
package boardview

import (
	"fmt"
	"image"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// rail is the board's panel with the parts under it.
func (p *Panel) rail(t *theme.Theme, gtx layout.Context, b hw.Board,
	st *state.NodeStat, rows []Row, s *state.Snapshot) layout.Dimensions {

	return onPanel(t, gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(t.Sp.S).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !hasPanel(b) {
						// Said rather than left blank: nobody has established
						// what this board carries, which is a different fact
						// from a board that carries nothing.
						return comp.Text(t, t.Sz.Caption, t.P.Faint,
							"nothing is recorded about what this board carries")(gtx)
					}
					return p.panelHead(t, gtx, b)
				}),
				// Above the panel rather than under it. Under it, the note is
				// the first thing pushed off the bottom when the panel is
				// turned up - which is exactly when what it says, the scale
				// and whether the board is powered, is worth reading.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !hasPanel(b) {
						return layout.Dimensions{}
					}
					return p.panelNote(t, gtx, b, st)
				}),
				// Lamps above the panel and the things somebody can press below
				// it, which follows from the kind rather than from a position:
				// none of this is a photograph, and a schematic that reads
				// correctly is worth more than a likeness.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					// "no lamp declared" is a fact about a board somebody has
					// looked at. On one nobody has, the line above already says
					// so and this would be a second, narrower claim we cannot
					// support.
					if !hasPanel(b) {
						return layout.Dimensions{}
					}
					return layout.Inset{Bottom: t.Sp.XXS}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return p.parts.Lamps(t, gtx, b.Hardware)
						})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !hasPanel(b) {
						return layout.Dimensions{}
					}
					return p.screen.Layout(t, gtx, b, st, p.scale, p.OnDo, p.Node, &p.pic)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !hasPanel(b) {
						return layout.Dimensions{}
					}
					return p.controls(t, gtx, b, s)
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
	// What it is doing goes on its own line rather than the end of that one.
	// A rail is only as wide as its panel, and a 240-wide one cut the state off
	// mid-word - which is the half a reader actually needs.
	doing := ""
	switch {
	case !engine.ScreenModelled(b):
		// The panel is real and transcribed from the board's own variant; the
		// emulator running it has nothing to draw on it. Said here as well as
		// in the table, because this is where somebody looks first.
		doing = "no display modelled"
	case st == nil || !st.Running:
		doing = "not powered"
	case st.Screen == nil:
		doing = "nothing drawn yet"
	case !st.Screen.On:
		// Not a fault: the firmware switches the panel off after an idle.
		doing = "asleep"
	}
	return layout.Inset{Bottom: t.Sp.XXS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(comp.OneLine(t, t.Sz.Caption, t.P.Faint, note, true)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if doing == "" {
						return layout.Dimensions{}
					}
					return comp.OneLine(t, t.Sz.Caption, t.P.Faint, doing, true)(gtx)
				}),
			)
		})
}

// partsIndex is the rows as a list to pick from, grouped.
func (p *Panel) partsIndex(t *theme.Theme, gtx layout.Context, rows []Row) layout.Dimensions {
	items := indexItems(rows)
	return comp.List(t, &p.index, len(items), func(gtx layout.Context, i int) layout.Dimensions {
		it := items[i]
		if it.head != "" {
			return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XXS}.Layout(gtx,
				comp.Text(t, t.Sz.Caption, t.P.Faint, upper(it.head)))
		}
		r := rows[it.row]
		sel := it.row == p.sel
		ink := t.P.Dim
		if sel {
			ink = t.P.Ink
		}
		// The same clickable the table row uses, so picking a part here and
		// picking its row over there are one act rather than two that have to
		// be kept in step.
		click := p.pick(p.rowKey(r))
		if click.Clicked(gtx) {
			p.sel = it.row
		}
		return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.XXS, Bottom: t.Sp.XXS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					if sel || click.Hovered() {
						fill := t.P.Selected
						if !sel {
							fill = t.P.Sunk
						}
						comp.RoundRect(gtx, image.Pt(gtx.Constraints.Max.X,
							gtx.Dp(unit.Dp(18))), 4, fill)
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
	})(gtx)
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
