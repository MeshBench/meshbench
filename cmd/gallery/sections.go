// The gallery's two columns, kept apart from the window plumbing.
package main

import (
	"fmt"
	"image"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

func (g *gallery) leftColumn(gtx layout.Context) layout.Dimensions {
	t := g.t
	return comp.List(t, &g.scroll, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(comp.SectionTitle(t, "Buttons")),
			layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return g.primary.Layout(t, gtx) }),
					layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return g.secondary.Layout(t, gtx) }),
					layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return g.quiet.Layout(t, gtx) }),
				)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return g.destructive.Layout(t, gtx) }),
					layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return g.disabled.Layout(t, gtx) }),
				)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
				"a disabled control carries its reason: "+g.disabled.Reason)),

			layout.Rigid(layout.Spacer{Height: t.Sp.XL}.Layout),
			layout.Rigid(comp.SectionTitle(t, "Fields")),
			layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return g.name.Layout(t, gtx) }),
			layout.Rigid(layout.Spacer{Height: t.Sp.M}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return g.freq.Layout(t, gtx) }),

			layout.Rigid(layout.Spacer{Height: t.Sp.XL}.Layout),
			layout.Rigid(comp.SectionTitle(t, "Switches")),
			layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return g.check1.Layout(t, gtx) }),
			layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return g.check2.Layout(t, gtx) }),

			layout.Rigid(layout.Spacer{Height: t.Sp.XL}.Layout),
			layout.Rigid(comp.SectionTitle(t, "Node kinds")),
			layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return g.kinds(gtx) }),

			layout.Rigid(layout.Spacer{Height: t.Sp.XL}.Layout),
			layout.Rigid(comp.SectionTitle(t, "Semantic colour")),
			layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return g.semantic(gtx) }),
		)
	})(gtx)
}

func (g *gallery) kinds(gtx layout.Context) layout.Dimensions {
	t := g.t
	names := []struct {
		k theme.NodeKind
		s string
	}{
		{theme.SimpleRepeater, "repeater"},
		{theme.AdvancedRepeater, "advanced"},
		{theme.Companion, "companion"},
		{theme.RoomServer, "room server"},
		{theme.Observer, "observer"},
		{theme.Emitter, "emitter"},
	}
	children := make([]layout.FlexChild, 0, len(names))
	for _, n := range names {
		n := n
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: t.Sp.M, Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							d := gtx.Dp(10)
							return comp.FillRect(gtx, image.Pt(d, d), t.NodeColour(n.k))
						}),
						layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout),
						layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, n.s)),
					)
				})
		}))
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

func (g *gallery) semantic(gtx layout.Context) layout.Dimensions {
	t := g.t
	items := []struct {
		c   [4]uint8
		s   string
		sub string
	}{
		{comp.Tint(t.P.Good), "good", "delivery held"},
		{comp.Tint(t.P.Warn), "warn", "duty near the limit"},
		{comp.Tint(t.P.Bad), "bad", "reach fell 18 points"},
		{comp.Tint(t.P.Accent), "accent", "selected"},
	}
	children := make([]layout.FlexChild, 0, len(items))
	for _, it := range items {
		it := it
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: t.Sp.L}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return comp.FillRect(gtx, image.Pt(gtx.Dp(64), gtx.Dp(6)), nrgbaOf(it.c))
					}),
					layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
					layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Dim, it.s)),
					layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, it.sub)),
				)
			})
		}))
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

func (g *gallery) rightColumn(gtx layout.Context) layout.Dimensions {
	t := g.t
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "Table")),
		layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
			"virtualised, sortable on a total order, filterable, with stable row identity")),
		layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			f := comp.Field{Hint: "filter by name, kind or region"}
			f.Editor = g.table.Filter
			d := f.Layout(t, gtx)
			g.table.Filter = f.Editor
			g.table.SetRows(demoRows(t))
			return d
		}),
		layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return g.table.Layout(t, gtx, func(key string) { g.table.Selected = key })
		}),
		layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return comp.Mono(t, t.Sz.Caption, t.P.Faint,
				fmt.Sprintf("%d of %d nodes", g.table.Shown(), len(demoRows(t))))(gtx)
		}),
	)
}

func (g *gallery) statusBar(gtx layout.Context) layout.Dimensions {
	t := g.t
	comp.Fill(gtx, t.P.Panel)
	return comp.Inset(t, t.Sp.S, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
				"every colour and measurement here comes from internal/gui/theme")),
			layout.Flexed(1, comp.Spacer),
			layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Faint, "Gio v0.10.2")),
		)
	})(gtx)
}
