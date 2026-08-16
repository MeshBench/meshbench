// The Journey tab: the message across every relay, and the toolbar that
// decides how the graph above the table is drawn.
package workbench

import (
	"fmt"
	"strings"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// graphToolbarTop: fold the graph away, pick its shape, reset the view, and
// say what it contains - the controls that matter whether or not the graph
// is even showing.
func (p *packetPanel) graphToolbarTop(t *theme.Theme, gtx layout.Context, g hopGraph) layout.Dimensions {
	kids := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.graphBtn.Layout(t, gtx, "graph", "", !p.noGraph, t.P.Accent)
		}),
	}
	if !p.noGraph {
		for i, gm := range graphModes {
			i, gm := i, gm
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: t.Sp.XXS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return p.modeChips[i].Layout(t, gtx, gm.Label, "", p.gview.mode == gm.Mode, t.P.Accent)
				})
			}))
		}
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return p.resetBtn.Layout(t, gtx)
			})
		}))
	}
	kids = append(kids,
		layout.Flexed(1, comp.Spacer),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, graphCaption(g))),
	)
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
}

// graphToolbarBottom: how deep the picture goes. Its own row rather than
// crowded onto the first one - three layout names plus five hop depths plus
// a caption does not fit one line in a panel docked to a third of a window.
func (p *packetPanel) graphToolbarBottom(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	kids := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: t.Sp.S}.Layout(gtx,
				comp.Text(t, t.Sz.Caption, t.P.Faint, "hops"))
		}),
	}
	for i, n := range hopLimits {
		i, n := i, n
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.hopChips[i].Layout(t, gtx, hopLimitLabel(n), "", p.gview.maxHops == n, t.P.Accent)
		}))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
}

// journey: the message across every relay - heard and missed, named.
func (p *packetPanel) journey(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
	type jRow struct {
		at    uint32
		hop   int
		names string
		heard bool
	}
	var rows []jRow
	for _, h := range pk.Journey {
		if len(h.Heard) > 0 {
			rows = append(rows, jRow{h.AtMs, h.Hops, nameList(h.Heard, 4), true})
		}
		if len(h.MissedBy) > 0 {
			rows = append(rows, jRow{h.AtMs, h.Hops, nameList(h.MissedBy, 4), false})
		}
		if len(h.Heard) == 0 && len(h.MissedBy) == 0 {
			rows = append(rows, jRow{h.AtMs, h.Hops, "nobody in range", false})
		}
	}
	head := func(w int, s string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			px := gtx.Dp(unitDp(w))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = px, px
			d := comp.Text(t, t.Sz.Caption, t.P.Faint, s)(gtx)
			d.Size.X = px
			return d
		})
	}
	g := buildHopGraph(pk, p.gview.maxHops, p.showKind)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return comp.CellGrid(t, gtx, 220, []layout.Widget{
				statBox(t, "heard", fmt.Sprintf("%d", pk.Heard),
					fmt.Sprintf("%d distinct nodes over the whole journey", pk.Reached)),
				statBox(t, "missed", fmt.Sprintf("%d", pk.Missed),
					"decode failures across every relay"),
			})
		}),
		// The propagation, drawn. Above the table rather than instead of it:
		// the picture answers what shape, the rows answer what exactly, and
		// somebody chasing a packet wants both within one glance.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return p.graphToolbarTop(t, gtx, g)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if p.noGraph {
								return layout.Dimensions{}
							}
							return layout.Inset{Top: t.Sp.XXS}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									return p.graphToolbarBottom(t, gtx)
								})
						}),
					)
				})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.noGraph || len(g.Nodes) == 0 {
				return layout.Dimensions{}
			}
			// Half the window, not half of whatever the toolbar and stat
			// boxes above it left over - see drawHopGraph.
			return drawHopGraph(t, gtx, g, &p.gview, p.winH/2)
		}),
		// The key. Every colour in the picture means something, and the reason
		// a hop failed is what an operator would act on - too weak wants an
		// antenna, lost to a stronger signal wants less traffic, deaf wants
		// different timing. Each entry is also the filter for its own class.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.noGraph || len(g.Nodes) == 0 {
				return layout.Dimensions{}
			}
			var kids []layout.FlexChild
			for i, mk := range missKinds {
				i, mk := i, mk
				n := g.Total[mk.Kind]
				if n == 0 {
					continue
				}
				col, _, _ := colourOf(t, mk.Kind)
				kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: t.Sp.XS}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return p.keyChips[i].Layout(t, gtx, mk.Label,
								fmt.Sprintf("%d", n), p.showKind[mk.Kind], col)
						})
				}))
			}
			if len(kids) == 0 {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
				})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.S, Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{}.Layout(gtx,
						head(70, "Time"), head(50, "Hop"),
						layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, "Heard by / Missed by")),
						layout.Flexed(1, comp.Spacer),
						layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, "Result")),
					)
				})
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return comp.List(t, &p.jList, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
				r := rows[i]
				word, col := "missed", t.P.Bad
				if r.heard {
					word, col = "heard", t.P.Good
				}
				return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								px := gtx.Dp(70)
								gtx.Constraints.Min.X, gtx.Constraints.Max.X = px, px
								d := comp.Mono(t, t.Sz.Caption, t.P.Dim,
									fmt.Sprintf("%.2f s", float64(r.at)/1000))(gtx)
								d.Size.X = px
								return d
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								px := gtx.Dp(50)
								gtx.Constraints.Min.X, gtx.Constraints.Max.X = px, px
								d := comp.Mono(t, t.Sz.Caption, t.P.Dim,
									fmt.Sprintf("%d", r.hop))(gtx)
								d.Size.X = px
								return d
							}),
							layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Ink, r.names, false)),
							layout.Rigid(comp.Text(t, t.Sz.Caption, col, word)),
						)
					})
			})(gtx)
		}),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, fmt.Sprintf(
			"%d transmissions, sorted by time - one message, followed by its "+
				"payload; the bytes differ at every hop", pk.Transmissions))),
	)
}

// nameList joins the first few names and counts the rest.
func nameList(names []string, keep int) string {
	if len(names) <= keep {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:keep], ", ") + fmt.Sprintf(", +%d more", len(names)-keep)
}
