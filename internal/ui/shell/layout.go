// Laying the panels out: how a view splits into rows, how a row splits into
// panels, and how big each one ends up.
package shell

import (
	"fmt"
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/app/version"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

func (sh *Shell) body(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	a := sh.arrangement()
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

// stack draws the rail's regions down a column, sharing the height equally.
func (sh *Shell) stack(t *theme.Theme, gtx layout.Context, s *state.Snapshot, cols []Col) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(cols)*2)
	for i := range cols {
		i := i
		if i > 0 {
			children = append(children, layout.Rigid(comp.HRule(t)))
		}
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return sh.region(t, gtx, s, regionRef{Row: -1, Col: i})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// rows draws the bands of a view, top to bottom, each taking its share.
func (sh *Shell) rows(t *theme.Theme, gtx layout.Context, s *state.Snapshot, rs []Row) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(rs)*2)
	for i := range rs {
		r, i := rs[i], i
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
				return sh.measuredRow(t, gtx, s, i, r.Cols, key)
			}))
			continue
		}
		w := r.Weight
		if w <= 0 {
			w = 1
		}
		children = append(children, layout.Flexed(float32(w), func(gtx layout.Context) layout.Dimensions {
			return sh.measuredRow(t, gtx, s, i, r.Cols, key)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// measuredRow draws a row and remembers how tall it came out, so a splitter
// above the next one has somewhere to start from.
func (sh *Shell) measuredRow(t *theme.Theme, gtx layout.Context, s *state.Snapshot,
	rowIdx int, cols []Col, key string) layout.Dimensions {
	d := sh.row(t, gtx, s, rowIdx, cols)
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
func (sh *Shell) row(t *theme.Theme, gtx layout.Context, s *state.Snapshot,
	rowIdx int, cols []Col) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(cols)*2)
	for j := range cols {
		c, j := cols[j], j
		ref := regionRef{Row: rowIdx, Col: j}
		if c.WidthDp > 0 {
			// Keyed by where the region is rather than by what is in it: a
			// region holds several panels now, and its width belongs to the
			// region rather than to whichever tab is showing.
			key := "col:" + regionKey(sh.View, ref)
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				w := gtx.Dp(unit.Dp(sh.sizeOf(key, c.WidthDp)))
				gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
				return sh.region(t, gtx, s, ref)
			}))
			// The rule after a fixed column sizes it: dragging right widens it.
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return sh.split(t, gtx, key, true, +1, c.WidthDp)
			}))
			continue
		}
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return sh.region(t, gtx, s, ref)
		}))
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

// region draws one region: its tabs, and the body of whichever is showing.
func (sh *Shell) region(t *theme.Theme, gtx layout.Context, s *state.Snapshot,
	ref regionRef) layout.Dimensions {
	comp.Fill(gtx, t.P.Panel)
	c := sh.regionAt(sh.View, ref)
	if c == nil {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	name := c.shown()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return sh.tabStrip(t, gtx, ref)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Left: t.Sp.M, Right: t.Sp.M, Top: t.Sp.XS, Bottom: t.Sp.S,
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return sh.panelBody(t, gtx, s, name)
			})
		}),
	)
}

// panelBody draws one panel's contents, or says why it is not drawing them.
func (sh *Shell) panelBody(t *theme.Theme, gtx layout.Context, s *state.Snapshot,
	name string) layout.Dimensions {
	if name == "" {
		// An empty region is an invitation, not a fault: it says what to do
		// with it rather than sitting blank.
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint,
			"empty - open a panel here from a menu"))
	}
	p := sh.Panels[name]
	if p == nil {
		return layout.Center.Layout(gtx,
			comp.Text(t, t.Sz.Caption, t.P.Faint, "not built yet"))
	}
	if sh.PoppedOut != nil && sh.PoppedOut(name) {
		// Said, not left blank: a panel that has gone somewhere looks
		// identical to one that has broken.
		return layout.Center.Layout(gtx,
			comp.Text(t, t.Sz.Caption, t.P.Dim, "in its own window"))
	}
	return p.Draw(t, gtx, s)
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
	// The physics, as a control rather than a caption: one click flips
	// between the two RF modes, and the label always says which one is
	// deciding reception right now.
	if sh.rfChip.Clicked(gtx) && sh.OnMenu != nil {
		sh.OnMenu("rf.toggle")
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		// A filled pill in the mode's own colour, so it reads as the
		// control it is: accent for the waveform physics, the muted panel
		// tone for calculated, and a press flips it.
		label, fill, fg := "calculated RF", t.P.Sunk, t.P.Ink
		if s != nil && s.RFMode == "waveform" {
			label, fill, fg = "waveform RF", t.P.Accent, t.P.Ground
		}
		if sh.rfChip.Hovered() {
			fill = theme.Alpha(fill, 0.85)
		}
		return layout.Inset{Left: t.Sp.S}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return sh.rfChip.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					// Centred by arithmetic, not by insets: the label is
					// measured, the pill is sized around it, and the label
					// is placed at the exact middle - an inset pair leaves
					// the text riding the line box's own slack, which reads
					// as a button somebody nudged.
					macro := op.Record(gtx.Ops)
					lbl := comp.Mono(t, t.Sz.Caption, fg, label)(gtx)
					call := macro.Stop()
					pad := gtx.Dp(t.Sp.S)
					// The pill hugs the glyphs, not the line box: the box
					// carries its slack under the baseline, and sizing or
					// centring by it hung the text high in a too-tall pill.
					// Capital height is ~0.72 of the point size; the pill is
					// that plus padding, and the label is placed so its
					// baseline - the one measurement the text reports - sits
					// where the glyphs come out visually centred.
					capPx := int(0.72 * float32(gtx.Sp(t.Sz.Caption)))
					size := image.Pt(lbl.Size.X+2*pad, capPx+2*gtx.Dp(4))
					r := unit.Dp(float32(size.Y) / gtx.Metric.PxPerDp / 2)
					comp.RoundRect(gtx, size, r, fill)
					comp.Border(gtx, size, r, 1, theme.Alpha(fg, 0.35))
					baseInBox := lbl.Size.Y - lbl.Baseline
					offY := size.Y/2 + capPx/2 - baseInBox
					off := op.Offset(image.Pt((size.X-lbl.Size.X)/2, offY)).Push(gtx.Ops)
					call.Add(gtx.Ops)
					off.Pop()
					return layout.Dimensions{Size: size}
				})
			})
	}))
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
	// How fast against the wall, measured rather than assumed: 1.00x is a
	// run keeping up, and the number sliding under it is the earliest
	// honest sign of a machine that is not.
	if s.Playing && s.RealtimeX > 0 {
		out += fmt.Sprintf("   %.2fx realtime", s.RealtimeX)
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
				// What this build is, in the corner a build identifier lives
				// in. Mono because a version is compared by eye against
				// another one, and faint because it is only ever wanted when
				// something has gone wrong.
				layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Faint, version.String()+" · Gio")),
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
