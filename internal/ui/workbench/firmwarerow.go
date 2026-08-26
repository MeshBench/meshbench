// One row of the firmware library, and the small pieces it draws with.
package workbench

import (
	"fmt"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

func (p *firmwarePanel) row(t *theme.Theme, gtx layout.Context, s *state.Snapshot,
	r state.FirmwareRow) layout.Dimensions {

	key := buildKey(r)
	w, ok := p.rows[key]
	if !ok {
		w = &fwRowW{}
		p.rows[key] = w
	}
	if w.use.Clicked(gtx) && p.OnAction != nil {
		p.OnAction("firmware.set", map[string]any{
			"role": r.Role, "version": r.Version,
		})
	}
	if w.open.Clicked(gtx) && p.OnAction != nil {
		// The window is opened by verb like everything else here, so that a
		// double-click and a script asking for it take the same path.
		p.OnAction("firmware.window", map[string]any{
			"version": r.Version, "role": r.Role, "board": r.Board,
		})
	}
	if w.act.Clicked(gtx) {
		switch {
		case r.Unavailable:
			// Nothing to download and nothing to delete.
		case !r.OnDisk:
			if p.OnAction != nil {
				p.OnAction("firmware.download", map[string]any{
					"role": r.Role, "version": r.Version, "board": r.Board,
				})
			}
		case p.confirm != key:
			// Deleting a build the scenario is using leaves those nodes unable
			// to start, and the failure arrives at play rather than here - so
			// the second press is the destructive one.
			p.confirm = key
		default:
			p.confirm = ""
			if p.OnAction != nil {
				p.OnAction("firmware.delete", map[string]any{
					"role": r.Role, "version": r.Version, "board": r.Board,
					"path": r.Path,
				})
			}
		}
	}

	cell := func(width int, wgt layout.Widget) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			px := gtx.Dp(unitDp(width))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = px, px
			d := wgt(gtx)
			// The column owns its width whatever the content drew: a tick is
			// twelve pixels, and without this everything after it slides left
			// and stops lining up with the headers.
			d.Size.X = px
			return d
		})
	}
	runsAs := "this machine"
	if r.Board != "" {
		runsAs = r.Board + " (emulated)"
	}
	// A row that exists only because nodes are pinned to it: no file, and
	// nothing published this machine could fetch. Saying so here is the
	// difference between "the library lost a build" and "these nodes are
	// pinned to something that was never built for you".
	if r.Unavailable {
		runsAs += " - not published for this machine"
	}
	size := "-"
	if r.OnDisk {
		size = fmt.Sprintf("%.1f MB", float64(r.Bytes)/(1<<20))
	}
	used := "-"
	if r.InUse > 0 {
		used = fmt.Sprintf("%d nodes", r.InUse)
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.S, Bottom: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						// The role and the version together are the way into the
						// build's own window: one clickable across both, because
						// two would mean the name and the icon behaved
						// differently from each other for no reason a reader
						// could see.
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return w.open.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								ink := t.P.Ink
								if w.open.Hovered() {
									ink = t.P.Accent
								}
								return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
									cell(fwCols[0].width, func(gtx layout.Context) layout.Dimensions {
										return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return roleIcon(t, gtx, r.Role)
											}),
											layout.Rigid(comp.OneLine(t, t.Sz.Caption, ink, r.Role, true)),
										)
									}),
									cell(fwCols[1].width, comp.OneLine(t, t.Sz.Caption, ink, r.Version, true)),
								)
							})
						}),
						cell(fwCols[2].width, comp.Text(t, t.Sz.Caption, t.P.Dim, runsAs)),
						cell(fwCols[3].width, comp.Mono(t, t.Sz.Caption, t.P.Dim, size)),
						cell(fwCols[4].width, func(gtx layout.Context) layout.Dimensions {
							if r.OnDisk {
								return tick(t, gtx)
							}
							return cross(t, gtx)
						}),
						cell(fwCols[5].width, comp.Mono(t, t.Sz.Caption, t.P.Dim, used)),
						cell(fwCols[6].width, func(gtx layout.Context) layout.Dimensions {
							return borderedAction(t, gtx, &w.use, "use for this role", t.P.Rule, t.P.Ink)
						}),
						cell(fwCols[7].width, func(gtx layout.Context) layout.Dimensions {
							label, line, ink := "delete", t.P.Rule, t.P.Dim
							if r.Unavailable {
								// Nothing to fetch, so no button that implies
								// there is.
								label, ink = "unavailable", t.P.Warn
							} else if !r.OnDisk {
								label, ink = "get", t.P.Accent
							} else if p.confirm == key {
								label, line, ink = "sure?", t.P.Bad, t.P.Bad
							}
							return borderedAction(t, gtx, &w.act, label, line, ink)
						}),
					)
				})
		}),
		layout.Rigid(comp.HRule(t)),
	)
}

func (p *firmwarePanel) header(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(comp.Text(t, t.Sz.Title, t.P.Ink, "Firmware Library")),
					layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
						"the builds this machine can run - what is in the cache is the "+
							"only thing that decides what a node can run")),
				)
			}),
			layout.Flexed(1, comp.Spacer),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: t.Sp.S}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions { return p.importBtn.Layout(t, gtx) })
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: t.Sp.S}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions { return p.refreshBtn.Layout(t, gtx) })
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.scanBtn.Layout(t, gtx)
			}),
		)
	})
}

func (p *firmwarePanel) tabRow(t *theme.Theme, gtx layout.Context, nAll, nDisk, nMachine, nBoards int) layout.Dimensions {
	chip := func(c *comp.Chip, label string, n, tab int) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return c.Layout(t, gtx, label, fmt.Sprintf("%d", n), p.tab == tab, t.P.Accent)
				})
		})
	}
	filterLabel := "filters"
	if p.roleFilter != "" {
		filterLabel = p.roleFilter
	}
	return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			chip(&p.tabAll, "All builds", nAll, 0),
			chip(&p.tabDisk, "On disk only", nDisk, 1),
			chip(&p.tabMachine, "This machine only", nMachine, 2),
			chip(&p.tabBoards, "Emulated boards", nBoards, 3),
			layout.Flexed(1, comp.Spacer),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = gtx.Dp(220)
				return layout.Inset{Right: t.Sp.S}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return p.search.Layout(t, gtx)
					})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.filterBtn.Layout(t, gtx, filterLabel, "", p.roleFilter != "", t.P.Accent)
			}),
		)
	})
}

func (p *firmwarePanel) colHeads(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	var kids []layout.FlexChild
	for _, c := range fwCols {
		c := c
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			w := gtx.Dp(unitDp(c.width))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
			d := comp.Text(t, t.Sz.Caption, t.P.Faint, c.label)(gtx)
			d.Size.X = w
			return d
		}))
	}
	return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx, kids...)
		})
}

func (p *firmwarePanel) footer(t *theme.Theme, gtx layout.Context, shown, all int) layout.Dimensions {
	return layout.Inset{Top: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
				fmt.Sprintf("showing %d of %d builds", shown, all))),
			layout.Flexed(1, comp.Spacer),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return tick(t, gtx) }),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, " on disk    ")),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return cross(t, gtx) }),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
				" not downloaded    delete asks twice")),
		)
	})
}

// roleIcon is a tinted rounded square with the role's mark: arcs for a
// companion's radio, a chip for everything that repeats.
func roleIcon(t *theme.Theme, gtx layout.Context, role string) layout.Dimensions {
	d := gtx.Dp(22)
	col := t.NodeColour(theme.SimpleRepeater)
	switch role {
	case "companion_radio":
		col = t.NodeColour(theme.Companion)
	case "advanced_repeater":
		col = t.NodeColour(theme.AdvancedRepeater)
	case "simple_room_server":
		col = t.NodeColour(theme.RoomServer)
	}
	rr := gtx.Dp(5)
	func() {
		defer clip.RRect{Rect: imageRectPt(d), NE: rr, NW: rr, SE: rr, SW: rr}.
			Push(gtx.Ops).Pop()
		paint.ColorOp{Color: theme.Alpha(col, 0.16)}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
	}()
	s := float32(d)
	stroke := func(spec clip.PathSpec) {
		paint.FillShape(gtx.Ops, col, clip.Stroke{Path: spec, Width: 1.4}.Op())
	}
	var pth clip.Path
	if role == "companion_radio" {
		// A dot with two arcs: a radio speaking.
		func() {
			r := int(s * 0.09)
			cx, cy := int(s*0.38), int(s*0.5)
			defer clip.Ellipse{Min: imagePtXY(cx-r, cy-r), Max: imagePtXY(cx+r, cy+r)}.
				Push(gtx.Ops).Pop()
			paint.ColorOp{Color: col}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
		}()
		for _, f := range []float32{0.22, 0.36} {
			pth = clip.Path{}
			pth.Begin(gtx.Ops)
			pth.MoveTo(f32.Pt(s*0.42+f*s*0.7, s*0.5-f*s))
			pth.QuadTo(f32.Pt(s*0.52+f*s*1.1, s*0.5), f32.Pt(s*0.42+f*s*0.7, s*0.5+f*s))
			stroke(pth.End())
		}
	} else {
		// A chip: a square with pins.
		pth = clip.Path{}
		pth.Begin(gtx.Ops)
		pth.MoveTo(f32.Pt(s*0.3, s*0.3))
		pth.LineTo(f32.Pt(s*0.7, s*0.3))
		pth.LineTo(f32.Pt(s*0.7, s*0.7))
		pth.LineTo(f32.Pt(s*0.3, s*0.7))
		pth.Close()
		stroke(pth.End())
		for _, f := range []float32{0.42, 0.58} {
			pth = clip.Path{}
			pth.Begin(gtx.Ops)
			pth.MoveTo(f32.Pt(s*f, s*0.18))
			pth.LineTo(f32.Pt(s*f, s*0.3))
			stroke(pth.End())
			pth = clip.Path{}
			pth.Begin(gtx.Ops)
			pth.MoveTo(f32.Pt(s*f, s*0.7))
			pth.LineTo(f32.Pt(s*f, s*0.82))
			stroke(pth.End())
		}
	}
	return layout.Dimensions{Size: imagePtXY(d+gtx.Dp(t.Sp.S), d)}
}

// tick and cross are the on-disk marks, and the footer's legend.
func tick(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	s := float32(gtx.Dp(12))
	var pth clip.Path
	pth.Begin(gtx.Ops)
	pth.MoveTo(f32.Pt(s*0.15, s*0.55))
	pth.LineTo(f32.Pt(s*0.4, s*0.8))
	pth.LineTo(f32.Pt(s*0.85, s*0.2))
	paint.FillShape(gtx.Ops, t.P.Good, clip.Stroke{Path: pth.End(), Width: 1.8}.Op())
	return layout.Dimensions{Size: imagePtXY(int(s), int(s))}
}

func cross(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	s := float32(gtx.Dp(12))
	for _, seg := range [][4]float32{{0.2, 0.2, 0.8, 0.8}, {0.8, 0.2, 0.2, 0.8}} {
		var pth clip.Path
		pth.Begin(gtx.Ops)
		pth.MoveTo(f32.Pt(s*seg[0], s*seg[1]))
		pth.LineTo(f32.Pt(s*seg[2], s*seg[3]))
		paint.FillShape(gtx.Ops, t.P.Bad, clip.Stroke{Path: pth.End(), Width: 1.8}.Op())
	}
	return layout.Dimensions{Size: imagePtXY(int(s), int(s))}
}
