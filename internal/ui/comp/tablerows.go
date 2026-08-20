// Drawing a table's header and rows, and the filtering that decides which
// rows there are.
package comp

import (
	"image"
	"math"
	"sort"
	"strconv"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

func (tb *Table) header(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(tb.Cols))
	for i := range tb.Cols {
		i := i
		c := tb.Cols[i]
		w := func(gtx layout.Context) layout.Dimensions {
			title := c.Title
			if tb.SortCol == i && c.Sortable {
				if tb.SortDesc {
					title += "  v"
				} else {
					title += "  ^"
				}
			}
			return tb.headers[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{
					Top: t.Sp.XS, Bottom: t.Sp.XS, Left: t.Sp.S, Right: t.Sp.S,
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if c.Right {
						return layout.E.Layout(gtx,
							Text(t, t.Sz.Caption, t.P.Faint, title))
					}
					return Text(t, t.Sz.Caption, t.P.Faint, title)(gtx)
				})
			})
		}
		if c.Width == 0 {
			children = append(children, layout.Flexed(1, w))
			continue
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			px := gtx.Dp(tb.widthOf(i))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = px, px
			return w(gtx)
		}))
		// A grip on the column's trailing edge. Dragging right widens it.
		//
		// Bounded to the header's own height. A splitter takes the height it is
		// offered, and inside a horizontal flex that is the whole panel - so an
		// unbounded grip claimed every click in its column all the way down the
		// table, and rows stopped selecting.
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.Y = gtx.Dp(t.RowHeight())
			d := tb.grips[i].Layout(t, gtx)
			if tb.grips[i].Delta != 0 {
				next := tb.widthOf(i) +
					unit.Dp(tb.grips[i].Delta/gtx.Metric.PxPerDp)
				if next < 40 {
					next = 40
				}
				tb.widths[i] = next
			}
			return d
		}))
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

func (tb *Table) row(t *theme.Theme, gtx layout.Context, idx int) layout.Dimensions {
	r := tb.shown[idx]
	if tb.OnRightClick != nil {
		tb.watchRightClick(gtx, r.Key)
	}
	selected := r.Key == tb.Selected
	return tb.rows[idx].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		h := gtx.Dp(t.RowHeight())
		macro := op.Record(gtx.Ops)
		children := make([]layout.FlexChild, 0, len(tb.Cols)+1)
		if r.Tint[3] > 0 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: t.Sp.S, Right: t.Sp.XS}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						d := gtx.Dp(8)
						return FillRect(gtx, image.Pt(d, d),
							nrgba(r.Tint))
					})
			}))
		}
		for i := range tb.Cols {
			i := i
			c := tb.Cols[i]
			cell := cellAt(r, i)
			w := func(gtx layout.Context) layout.Dimensions {
				fg := t.P.Ink
				if i > 0 {
					fg = t.P.Dim
				}
				render := OneLine(t, t.Sz.Body, fg, cell, false)
				if c.Mono {
					render = OneLine(t, t.Sz.Data, fg, cell, true)
				}
				if c.Menu {
					// A caret, so a cell that can be changed does not look
					// like one that cannot.
					inner := render
					ck := tb.cell(r.Key, i)
					render = func(gtx layout.Context) layout.Dimensions {
						if ck.Clicked(gtx) && tb.OnCell != nil {
							tb.OnCell(r.Key, i)
						}
						return ck.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							bg := theme.Alpha(t.P.Ink, 0.05)
							if ck.Hovered() {
								bg = theme.Alpha(t.P.Accent, 0.18)
							}
							FillRect(gtx, image.Pt(gtx.Constraints.Max.X,
								gtx.Dp(t.RowHeight())-gtx.Dp(4)), bg)
							return layout.Flex{}.Layout(gtx,
								layout.Flexed(1, inner),
								layout.Rigid(Text(t, t.Sz.Caption, t.P.Dim, " v")),
							)
						})
					}
				}
				return layout.Inset{Left: t.Sp.S, Right: t.Sp.S}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						if c.Right {
							return layout.E.Layout(gtx, render)
						}
						return render(gtx)
					})
			}
			if c.Width == 0 {
				children = append(children, layout.Flexed(1, w))
				continue
			}
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				px := gtx.Dp(tb.widthOf(i))
				gtx.Constraints.Min.X, gtx.Constraints.Max.X = px, px
				return w(gtx)
			}))
			// The width of the header's grip, so a cell sits under its own
			// title. Without it every column after the first slides left by a
			// handle's width per column, which reads as a table that cannot
			// count.
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(2*grab+1, 0)}
			}))
		}
		gtx.Constraints.Min.Y = h
		gtx.Constraints.Max.Y = h
		dims := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		call := macro.Stop()
		dims.Size.Y = h
		switch {
		case selected:
			FillRect(gtx, image.Pt(gtx.Constraints.Max.X, h), t.P.Selected)
		case tb.rows[idx].Hovered():
			tb.hovered = tb.shown[idx].Key
			FillRect(gtx, image.Pt(gtx.Constraints.Max.X, h), theme.Alpha(t.P.Ink, 0.05))
		}
		call.Add(gtx.Ops)
		return dims
	})
}

// empty says what to do next rather than "no data", which is the difference
// between an interface that is unhelpful and one that is broken-looking.
func (tb *Table) empty(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	msg := "nothing here yet"
	if strings.TrimSpace(tb.Filter.Text()) != "" {
		msg = "nothing matches that filter"
	}
	return layout.Center.Layout(gtx, Text(t, t.Sz.Body, t.P.Faint, msg))
}

// cell is the clickable for one interactive cell, kept by row key so it
// survives a re-sort - which is the same reason rows are keyed rather than
// indexed.
func (tb *Table) cell(key string, col int) *widget.Clickable {
	if tb.cells == nil {
		tb.cells = map[string]*widget.Clickable{}
	}
	k := key + "\x00" + string(rune('0'+col))
	c, ok := tb.cells[k]
	if !ok {
		c = &widget.Clickable{}
		tb.cells[k] = c
	}
	return c
}

func rowMatches(r Row, want string) bool {
	if strings.Contains(strings.ToLower(r.Key), want) {
		return true
	}
	for _, c := range r.Cells {
		if strings.Contains(strings.ToLower(c), want) {
			return true
		}
	}
	return false
}

func cellAt(r Row, i int) string {
	if i < 0 || i >= len(r.Cells) {
		return ""
	}
	return r.Cells[i]
}

// numericValue is the number a Numeric column's cell shows.
//
// Cells are formatted strings, so the value is read back out of the format: a
// leading signed decimal, scaled by the SI magnitude prefix of whatever unit
// follows it, so "512 kB" and "4.2 MB" compare as bytes rather than as
// digits. NaN means the cell shows no number at all.
func numericValue(s string) float64 {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) {
		c := s[end]
		if c >= '0' && c <= '9' || c == '.' || (end == 0 && (c == '-' || c == '+')) {
			end++
			continue
		}
		break
	}
	v, err := strconv.ParseFloat(s[:end], 64)
	if err != nil {
		return math.NaN()
	}
	// A magnitude prefix only counts with a unit behind it - the k in "kB"
	// and "km", never a lone letter, and never the s in "s" or the % in "%".
	if unit := strings.TrimSpace(s[end:]); len(unit) >= 2 {
		switch unit[0] {
		case 'k':
			v *= 1e3
		case 'M':
			v *= 1e6
		case 'G':
			v *= 1e9
		case 'T':
			v *= 1e12
		case 'P':
			v *= 1e15
		}
	}
	return v
}

// search is the filter box above the header.
//
// Its hint says what it matches on rather than "search", because a box that
// does not say what it looks at gets tried once with the wrong thing.
func (tb *Table) search(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	tb.Filter.SingleLine = true
	hint := tb.FilterHint
	if hint == "" {
		hint = "filter - matches any column"
	}
	ed := material.Editor(t.M, &tb.Filter, hint)
	ed.Color = t.P.Ink
	ed.HintColor = t.P.Faint
	ed.TextSize = t.Sz.Body
	return layout.Inset{Left: t.Sp.S, Right: t.Sp.S,
		Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx, ed.Layout)
}

// ShownKeys is the set of rows surviving the filter, so a caller can total
// exactly what is on screen rather than what it handed over.
func (tb *Table) ShownKeys() map[string]bool {
	out := make(map[string]bool, len(tb.shown))
	for _, r := range tb.shown {
		out[r.Key] = true
	}
	return out
}

// applyFilter rebuilds the shown set from the rows last given.
func (tb *Table) applyFilter() {
	rows := tb.all
	want := strings.ToLower(strings.TrimSpace(tb.Filter.Text()))
	tb.lastFilter = want
	tb.shown = tb.shown[:0]
	for _, r := range rows {
		if want == "" || rowMatches(r, want) {
			tb.shown = append(tb.shown, r)
		}
	}
	col := tb.SortCol
	if col >= 0 && col < len(tb.Cols) {
		numeric := tb.Cols[col].Numeric
		// A total order: the sort key first, then the row's own key. Without
		// the second term, rows with equal values are free to swap places on
		// every sort, which is what made the old table shimmer.
		sort.SliceStable(tb.shown, func(i, j int) bool {
			a, b := cellAt(tb.shown[i], col), cellAt(tb.shown[j], col)
			if numeric {
				av, bv := numericValue(a), numericValue(b)
				// A dash sorts after every number in either direction:
				// "no data" is an absence, not a value below zero.
				switch {
				case math.IsNaN(av) && math.IsNaN(bv):
					return tb.shown[i].Key < tb.shown[j].Key
				case math.IsNaN(av):
					return false
				case math.IsNaN(bv):
					return true
				}
				if av == bv {
					return tb.shown[i].Key < tb.shown[j].Key
				}
				if tb.SortDesc {
					return av > bv
				}
				return av < bv
			}
			if a == b {
				return tb.shown[i].Key < tb.shown[j].Key
			}
			if tb.SortDesc {
				return a > b
			}
			return a < b
		})
	}
	for len(tb.rows) < len(tb.shown) {
		tb.rows = append(tb.rows, widget.Clickable{})
	}
	for len(tb.headers) < len(tb.Cols) {
		tb.headers = append(tb.headers, widget.Clickable{})
	}
}

// watchRightClick reports a secondary press on this row.
//
// Separate from the row's Clickable because Gio's clickable is about the
// primary button, and a right-click that also selected the row would make
// "open the menu" and "change the selection" the same gesture.
func (tb *Table) watchRightClick(gtx layout.Context, key string) {
	tag := tb.cell(key, 31) // a tag per row, distinct from any cell's
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, tag)
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press})
		if !ok {
			return
		}
		e, ok := ev.(pointer.Event)
		if ok && e.Buttons.Contain(pointer.ButtonSecondary) {
			tb.OnRightClick(key, e.Position)
		}
	}
}
