// Laying the panels out: how a view splits into rows, how a row splits into
// panels, and how big each one ends up.
package shell

import (
	"fmt"
	"gioui.org/layout"
	"gioui.org/unit"
	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

func (sh *Shell) body(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	a := arrangementFor(sh.View)
	if len(a.Rail) == 0 {
		return sh.rows(t, gtx, s, a.Rows)
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return sh.rows(t, gtx, s, a.Rows)
		}),
		// Dragged leftwards the rail grows, so the delta is subtracted.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return sh.split(t, gtx, "rail:"+sh.View.String(), true, -1, a.RailDp)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			w := gtx.Dp(unit.Dp(sh.sizeOf("rail:"+sh.View.String(), a.RailDp)))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
			return sh.stack(t, gtx, s, a.Rail)
		}),
	)
}

// sizeOf is a panel dimension as it stands: what the arrangement declared,
// plus whatever the operator has dragged it to since.
func (sh *Shell) sizeOf(key string, declared int) int {
	if sh.sizes == nil {
		return declared
	}
	if v, ok := sh.sizes[key]; ok {
		return v
	}
	return declared
}

// split draws a draggable rule and folds the drag into the stored size.
//
// sign is which way the neighbour grows: a rule to the left of what it sizes
// grows it when dragged right, one to the right of it does the opposite.
func (sh *Shell) split(t *theme.Theme, gtx layout.Context, key string, vertical bool, sign, declared int) layout.Dimensions {
	if sh.splitters == nil {
		sh.splitters = map[string]*comp.Splitter{}
	}
	sp := sh.splitters[key]
	if sp == nil {
		sp = &comp.Splitter{Vertical: vertical}
		sh.splitters[key] = sp
	}
	d := sp.Layout(t, gtx)
	if sp.Delta != 0 {
		if sh.sizes == nil {
			sh.sizes = map[string]int{}
		}
		// Stored in dp, because that is what the arrangement speaks and what
		// survives a window moving to a display of another density.
		next := sh.sizeOf(key, declared) +
			sign*int(sp.Delta/float32(gtx.Metric.PxPerDp))
		if next < 160 {
			next = 160 // below this a panel is a title and nothing else
		}
		sh.sizes[key] = next
	}
	return d
}

// stack draws panels down a column, sharing the height equally.
func (sh *Shell) stack(t *theme.Theme, gtx layout.Context, s *state.Snapshot, names []string) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(names)*2)
	for i, n := range names {
		n := n
		if i > 0 {
			children = append(children, layout.Rigid(comp.HRule(t)))
		}
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return sh.panel(t, gtx, s, n)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// rows draws the bands of a view, top to bottom, each taking its share.
func (sh *Shell) rows(t *theme.Theme, gtx layout.Context, s *state.Snapshot, rs []Row) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(rs)*2)
	for i, r := range rs {
		r, i := r, i
		key := "row:" + sh.View.String() + ":" + itoa(i)
		if i > 0 {
			// The rule above a row sizes the row before it: dragging down
			// gives that one more height. Its baseline is the height it took
			// last frame, because a row is declared as a share and a share has
			// no dp until something has laid it out.
			prev := "row:" + sh.View.String() + ":" + itoa(i-1)
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return sh.split(t, gtx, prev, false, +1, sh.rowDp(prev))
			}))
		}
		// The last row always takes what is left. Fixing every row would leave
		// a gap or overflow the window the moment it is resized.
		if h, ok := sh.sizes[key]; ok && i < len(rs)-1 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				px := gtx.Dp(unit.Dp(h))
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = px, px
				return sh.measuredRow(t, gtx, s, r.Cols, key)
			}))
			continue
		}
		w := r.Weight
		if w <= 0 {
			w = 1
		}
		children = append(children, layout.Flexed(float32(w), func(gtx layout.Context) layout.Dimensions {
			return sh.measuredRow(t, gtx, s, r.Cols, key)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// measuredRow draws a row and remembers how tall it came out, so a splitter
// above the next one has somewhere to start from.
func (sh *Shell) measuredRow(t *theme.Theme, gtx layout.Context, s *state.Snapshot,
	cols []Col, key string) layout.Dimensions {
	d := sh.row(t, gtx, s, cols)
	if gtx.Metric.PxPerDp > 0 && d.Size.Y > 0 {
		if sh.lastRow == nil {
			sh.lastRow = map[string]int{}
		}
		sh.lastRow[key] = int(float32(d.Size.Y) / gtx.Metric.PxPerDp)
	}
	return d
}

// rowDp is what a row took last frame, in dp.
func (sh *Shell) rowDp(key string) int {
	if v, ok := sh.lastRow[key]; ok && v > 0 {
		return v
	}
	return 300
}

// row draws one band's panels side by side. A column with a width keeps it; the
// rest share what is left.
func (sh *Shell) row(t *theme.Theme, gtx layout.Context, s *state.Snapshot, cols []Col) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(cols)*2)
	for _, c := range cols {
		c := c
		if c.WidthDp > 0 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				w := gtx.Dp(unit.Dp(sh.sizeOf("col:"+c.Name, c.WidthDp)))
				gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
				return sh.panel(t, gtx, s, c.Name)
			}))
			// The rule after a fixed column sizes it: dragging right widens it.
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return sh.split(t, gtx, "col:"+c.Name, true, +1, c.WidthDp)
			}))
			continue
		}
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return sh.panel(t, gtx, s, c.Name)
		}))
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
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
	out := itoa(len(s.Nodes)) + " nodes   seed " + itoa64(s.Seed) +
		"   t = " + msToS(s.NowMs)
	// Which physics is deciding reception, whenever it is not the default:
	// a run whose results came from the waveform must say so everywhere the
	// operator is already looking.
	if s.RFMode == "waveform" {
		out += "   waveform RF"
	}
	return out
}

func (sh *Shell) statusBar(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	comp.Fill(gtx, t.P.Panel)
	msg := sh.View.Purpose()
	if s != nil && s.Status != "" {
		msg = s.Status
	}
	// A running job owns the line: a long raster or pull that says nothing
	// reads as a hang, and the percentage is the difference between "wait"
	// and "force-quit".
	if s != nil {
		for i := range s.Jobs {
			if s.Jobs[i].Finished {
				continue
			}
			j := &s.Jobs[i]
			if j.Total > 0 {
				msg = fmt.Sprintf("%s - %d%% (%d of %d)",
					j.What, j.Done*100/j.Total, j.Done, j.Total)
			} else {
				msg = j.What + " - working"
			}
			break
		}
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
