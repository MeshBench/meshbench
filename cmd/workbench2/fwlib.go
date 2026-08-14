// The Firmware Library, to Alex's mock: a page, not a spreadsheet.
//
// A header that says what the page is for with the actions beside it, counted
// tabs for the three views, search and a role filter, and generous rows - a
// tinted role icon, the version, how it runs, its size, a tick for on disk,
// who uses it, a bordered use-for-role control and a delete that asks twice.
// The footer says how much is shown and what the marks mean. Deliberately no
// "Latest" mark: version names here include branches and study builds, and a
// chip that guesses wrong crowns somebody's experiment - Alex's call.
package main

import (
	"fmt"
	"image"
	"image/color"
	"sort"
	"strings"

	"gioui.org/f32"
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

// Small aliases that keep the drawing code readable.
type colorNRGBA = color.NRGBA

func unitDp(v int) unit.Dp                       { return unit.Dp(v) }
func imagePtXY(x, y int) image.Point             { return image.Pt(x, y) }
func imageRectPt(d int) image.Rectangle          { return image.Rect(0, 0, d, d) }
func imageRectSz(sz image.Point) image.Rectangle { return image.Rectangle{Max: sz} }

type fwRowW struct {
	use, act widget.Clickable
}

type firmwarePanel struct {
	tabAll, tabDisk, tabMachine comp.Chip
	tab                         int
	search                      comp.Field
	filterBtn                   comp.Chip
	roleFilter                  string
	importBtn                   comp.Button
	refreshBtn                  comp.Button
	scanBtn                     comp.Button
	list                        widget.List
	rows                        map[string]*fwRowW
	confirm                     string
	built                       bool
	asked                       bool
	// OnAction sends a verb about one build; Refresh asks for the library
	// again; OnImport asks for a path and a role; choose opens the chooser.
	OnAction func(verb string, params map[string]any)
	Refresh  func()
	OnImport func()
	choose   func(title string, opts []string, pick func(string))
}

func (p *firmwarePanel) build() {
	p.importBtn.Label, p.importBtn.Kind = "import...", comp.Secondary
	p.refreshBtn.Label, p.refreshBtn.Kind = "refresh", comp.Secondary
	p.scanBtn.Label, p.scanBtn.Kind = "scan builds", comp.Primary
	p.search.Hint = "search builds..."
	p.search.Editor.SingleLine = true
	p.rows = map[string]*fwRowW{}
	p.list.Axis = layout.Vertical
	p.built = true
}

// The column grid, one place: label and width per column, zero meaning "the
// rest".
var fwCols = []struct {
	label string
	width int
}{
	{"Role", 210}, {"Version", 250}, {"Runs as", 140}, {"Size", 70},
	{"On disk", 70}, {"Used by", 90}, {"Use for role", 150}, {"Actions", 70},
}

func (p *firmwarePanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.built {
		p.build()
	}
	if !p.asked && p.Refresh != nil {
		p.asked = true
		p.Refresh()
	}
	if p.tabAll.Click.Clicked(gtx) {
		p.tab = 0
	}
	if p.tabDisk.Click.Clicked(gtx) {
		p.tab = 1
	}
	if p.tabMachine.Click.Clicked(gtx) {
		p.tab = 2
	}
	if p.importBtn.Click.Clicked(gtx) && p.OnImport != nil {
		p.OnImport()
	}
	if p.refreshBtn.Click.Clicked(gtx) && p.Refresh != nil {
		p.Refresh()
	}
	if p.scanBtn.Click.Clicked(gtx) {
		if p.OnAction != nil {
			p.OnAction("firmware.published", nil)
		}
		if p.Refresh != nil {
			p.Refresh()
		}
	}
	if p.filterBtn.Click.Clicked(gtx) && p.choose != nil && s != nil {
		seen := map[string]bool{}
		opts := []string{"every role"}
		for i := range s.Library {
			if r := s.Library[i].Role; !seen[r] {
				seen[r] = true
				opts = append(opts, r)
			}
		}
		p.choose("Only this role", opts, func(picked string) {
			if picked == "every role" {
				picked = ""
			}
			p.roleFilter = picked
		})
	}
	if s == nil {
		return layout.Dimensions{}
	}

	nAll, nDisk, nMachine := 0, 0, 0
	for i := range s.Library {
		nAll++
		if s.Library[i].OnDisk {
			nDisk++
		}
		if s.Library[i].Board == "" {
			nMachine++
		}
	}

	want := strings.ToLower(fieldText(&p.search))
	shown := make([]state.FirmwareRow, 0, len(s.Library))
	for i := range s.Library {
		r := s.Library[i]
		if p.tab == 1 && !r.OnDisk {
			continue
		}
		if p.tab == 2 && r.Board != "" {
			continue
		}
		if p.roleFilter != "" && r.Role != p.roleFilter {
			continue
		}
		if want != "" && !strings.Contains(strings.ToLower(r.Role), want) &&
			!strings.Contains(strings.ToLower(r.Version), want) &&
			!strings.Contains(strings.ToLower(r.Board), want) {
			continue
		}
		shown = append(shown, r)
	}
	sort.Slice(shown, func(i, j int) bool {
		if shown[i].Role != shown[j].Role {
			return shown[i].Role < shown[j].Role
		}
		return shown[i].Version < shown[j].Version
	})

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.header(t, gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.tabRow(t, gtx, nAll, nDisk, nMachine)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.colHeads(t, gtx)
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(shown) == 0 {
				return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
					"nothing here - other tabs may hold builds, and scan builds asks the catalogue"))
			}
			return comp.List(t, &p.list, len(shown), func(gtx layout.Context, i int) layout.Dimensions {
				return p.row(t, gtx, s, shown[i])
			})(gtx)
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.footer(t, gtx, len(shown), nAll)
		}),
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

func (p *firmwarePanel) tabRow(t *theme.Theme, gtx layout.Context, nAll, nDisk, nMachine int) layout.Dimensions {
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
	for i, c := range fwCols {
		c := c
		if i == len(fwCols)-1 {
			kids = append(kids, layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, c.label)))
			break
		}
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			w := gtx.Dp(unitDp(c.width))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
			return comp.Text(t, t.Sz.Caption, t.P.Faint, c.label)(gtx)
		}))
	}
	return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx, kids...)
		})
}

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
	if w.act.Clicked(gtx) {
		switch {
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
				})
			}
		}
	}

	cell := func(width int, wgt layout.Widget) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			px := gtx.Dp(unitDp(width))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = px, px
			return wgt(gtx)
		})
	}
	runsAs := "this machine"
	if r.Board != "" {
		runsAs = r.Board + " (emulated)"
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
						cell(fwCols[0].width, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return roleIcon(t, gtx, r.Role)
								}),
								layout.Rigid(comp.OneLine(t, t.Sz.Caption, t.P.Ink, r.Role, true)),
							)
						}),
						cell(fwCols[1].width, comp.OneLine(t, t.Sz.Caption, t.P.Ink, r.Version, true)),
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
							if !r.OnDisk {
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

// borderedAction is the mock's per-row control: a bordered rounded box that
// reads as pressable without shouting like a primary button.
func borderedAction(t *theme.Theme, gtx layout.Context, ck *widget.Clickable,
	label string, line, ink colorNRGBA) layout.Dimensions {
	return ck.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if ck.Hovered() {
			ink = t.P.Ink
		}
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{
			Top: t.Sp.XS, Bottom: t.Sp.XS, Left: t.Sp.S, Right: t.Sp.S,
		}.Layout(gtx, comp.Text(t, t.Sz.Caption, ink, label))
		call := macro.Stop()
		comp.RoundRect(gtx, dims.Size, 5, theme.Alpha(t.P.Sunk, 0.6))
		comp.Border(gtx, dims.Size, 5, 1, line)
		call.Add(gtx.Ops)
		return dims
	})
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
