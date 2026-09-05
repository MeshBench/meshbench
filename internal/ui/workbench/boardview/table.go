// The middle: which table is showing, and the rows of it.
package boardview

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// middle is the tabs and the table under them.
func (p *Panel) middle(t *theme.Theme, gtx layout.Context, rows []Row) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.tabStrip(t, gtx)
		}),
		layout.Rigid(hRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(rows) == 0 {
				return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint,
					"this node's radio has not reported yet"))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.tableHead(t, gtx, rows)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					where := anyWhere(rows)
					return p.rows.Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
						return p.tableRow(t, gtx, rows[i], i, where)
					})
				}),
			)
		}),
	)
}

func (p *Panel) tabStrip(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	return layout.Inset{Left: t.Sp.S, Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			var kids []layout.FlexChild
			for i := range p.tabs {
				tb := Tab(i)
				ink := t.P.Faint
				if tb == p.Tab {
					ink = t.P.Ink
				}
				kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.tabs[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Right: t.Sp.M}.Layout(gtx,
							comp.Text(t, t.Sz.Caption, ink, tb.String()))
					})
				}))
			}
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
		})
}

// cols are the table's columns. One table of them so the head and the rows
// cannot drift: a header that has stopped matching its rows is a table that
// lies about every number under it.
var cols = []struct {
	w    int
	name string
}{
	{150, "WHAT"}, {105, "WHERE"}, {150, "DECLARED"}, {175, "OBSERVED"}, {125, "VERDICT"},
}

// anyWhere reports whether a table has anywhere to point at.
//
// The radio table does not: its rows are registers rather than pins, and a
// column of dashes down every one of them is a column that says nothing and
// takes the width the observed values need.
func anyWhere(rows []Row) bool {
	for _, r := range rows {
		if r.Where != "" {
			return true
		}
	}
	return false
}

func (p *Panel) tableHead(t *theme.Theme, gtx layout.Context, rows []Row) layout.Dimensions {
	var kids []layout.FlexChild
	for _, c := range cols {
		if c.name == "WHERE" && !anyWhere(rows) {
			continue
		}
		kids = append(kids, comp.Fixed(gtx, c.w,
			comp.Text(t, t.Sz.Caption, t.P.Faint, c.name)))
	}
	return layout.Inset{Left: t.Sp.S, Right: t.Sp.S, Bottom: t.Sp.XS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx, kids...)
		})
}

func (p *Panel) tableRow(t *theme.Theme, gtx layout.Context, r Row, i int, where bool) layout.Dimensions {
	sel := i == p.sel
	ink, dim := t.P.Dim, t.P.Faint
	if sel {
		ink, dim = t.P.Ink, t.P.Dim
	}
	obs := ink
	if r.Verdict == Diverged || r.Verdict == Silent {
		obs = r.Verdict.Colour(t)
	}
	cells := []struct {
		w int
		s string
		c color.NRGBA
	}{
		{150, r.Name, ink}, {105, orDash(r.Where), dim},
		{150, orDash(r.Declared), ink}, {175, orDash(r.Observed), obs},
		{125, r.Verdict.String(), r.Verdict.Colour(t)},
	}
	return layout.Inset{Left: t.Sp.S, Right: t.Sp.S, Top: t.Sp.XXS,
		Bottom: t.Sp.XXS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if sel {
			comp.RoundRect(gtx, image.Pt(gtx.Constraints.Max.X,
				gtx.Dp(unit.Dp(19))), 4, t.P.Selected)
		}
		var kids []layout.FlexChild
		for i, c := range cells {
			if i == 1 && !where {
				continue
			}
			kids = append(kids, comp.Fixed(gtx, c.w,
				comp.OneLine(t, t.Sz.Caption, c.c, c.s, true)))
		}
		return layout.Flex{}.Layout(gtx, kids...)
	})
}
