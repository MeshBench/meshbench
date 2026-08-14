package comp

import (
	"image"
	"sort"
	"strings"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// Column describes one column of a Table.
type Column struct {
	Title string
	// Width in Dp. Zero means "take the remaining space", and at most one
	// column should say so.
	Width unit.Dp
	// Right aligns numeric columns, which is what makes tabular figures worth
	// having.
	Right bool
	// Mono renders this column in the monospace face: identifiers, versions,
	// checksums and anything compared by eye down a column.
	Mono bool
	// Sortable columns get a header that responds to a click.
	Sortable bool
	// Menu makes this column's cells clickable, for a value that is chosen
	// rather than read. The table reports the click; what to show is the
	// caller's business, because the table does not know what the values are.
	Menu bool
}

// Row is one row's cells plus the identity the table sorts and selects by.
//
// Key is the point: a row is identified by something stable, not by its index,
// so a re-sort moves rows rather than renaming them. The old firmware table
// flickered every frame precisely because equal sort keys left the order free
// to change between frames.
type Row struct {
	Key   string
	Cells []string
	// Tint colours a small swatch at the start of the row, for node kind and
	// similar. Zero alpha means no swatch.
	Tint [4]uint8
}

// Table is a virtualised, sortable, selectable table.
//
// Virtualised: only the visible rows are built, so a 311-node network and a
// 20,000-event ledger cost the same per frame. The old table capped itself at
// 150 rows and said so, which is a worse answer than drawing what is asked for.
type Table struct {
	Cols     []Column
	Filter   widget.Editor
	List     widget.List
	SortCol  int
	SortDesc bool
	// FilterHint is what the search box says when it is empty.
	FilterHint string
	// OnCell is called when a Menu column's cell is clicked, with the row's
	// key and the column index.
	OnCell func(key string, col int)
	// OnRightClick is called when a row is right-clicked, with where. A
	// context menu is the one place an interface can offer everything about a
	// thing without spending a button on each, which matters when the thing is
	// a row and there are three hundred of them.
	OnRightClick func(key string, at f32.Point)
	cells        map[string]*widget.Clickable
	// Selected is the Key of the selected row, so selection survives a re-sort
	// and a filter change.
	Selected string

	headers []widget.Clickable
	// all is every row last handed over, before filtering.
	all []Row
	// lastFilter is the filter text the shown set was built with.
	lastFilter string
	rows       []widget.Clickable
	shown      []Row
}

// SetRows replaces the table's contents. Sorting and filtering are applied
// here rather than during layout, so a frame never does work proportional to
// the whole data set.
func (tb *Table) SetRows(rows []Row) {
	// Kept so the filter can be re-applied when somebody types, without the
	// caller having to hand the rows over again. Filtering used to happen
	// only here, and callers only call this when their data changes: typing
	// into the box did nothing at all until something else moved.
	tb.all = append(tb.all[:0], rows...)
	tb.applyFilter()
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
		// A total order: the sort key first, then the row's own key. Without
		// the second term, rows with equal values are free to swap places on
		// every sort, which is what made the old table shimmer.
		sort.SliceStable(tb.shown, func(i, j int) bool {
			a, b := cellAt(tb.shown[i], col), cellAt(tb.shown[j], col)
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

// Shown is how many rows survive the filter, for a caller that wants to say
// "12 of 311".
func (tb *Table) Shown() int { return len(tb.shown) }

// Layout draws the table. onSelect is called with a row key when a row is
// clicked, so selection is the caller's business rather than the widget's.
func (tb *Table) Layout(t *theme.Theme, gtx layout.Context, onSelect func(key string)) layout.Dimensions {
	tb.List.Axis = layout.Vertical
	// Re-filter when the box has been typed into. The editor changes on the
	// frame the character arrives; nothing else need happen for the list to
	// follow it.
	if want := strings.ToLower(strings.TrimSpace(tb.Filter.Text())); want != tb.lastFilter {
		tb.applyFilter()
	}
	// Sized here, where they are used, not only in SetRows. A table laid out
	// before its first SetRows - a panel drawn from a snapshot whose sequence
	// has not moved, or a window opened at the wrong moment - indexed a
	// zero-length slice and took the process with it.
	for len(tb.headers) < len(tb.Cols) {
		tb.headers = append(tb.headers, widget.Clickable{})
	}
	for i := range tb.headers {
		if tb.headers[i].Clicked(gtx) && tb.Cols[i].Sortable {
			if tb.SortCol == i {
				tb.SortDesc = !tb.SortDesc
			} else {
				tb.SortCol, tb.SortDesc = i, false
			}
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// The search box, which SetRows has always applied and nothing ever
		// drew. The filtering worked; there was simply no way to type into it,
		// so every table in the interface looked as though it had no search.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return tb.search(t, gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return tb.header(t, gtx) }),
		layout.Rigid(HRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(tb.shown) == 0 {
				return tb.empty(t, gtx)
			}
			// Clipped to the space the flex gave it.
			//
			// A scrolling list draws the row that is half off the bottom, and
			// without a clip that half lands on whatever the flex put beneath -
			// which is how the node view's total line ended up sitting on top
			// of its own last row.
			defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
			return material_List(t, &tb.List).Layout(gtx, len(tb.shown),
				func(gtx layout.Context, i int) layout.Dimensions {
					if tb.rows[i].Clicked(gtx) && onSelect != nil {
						onSelect(tb.shown[i].Key)
					}
					return tb.row(t, gtx, i)
				})
		}),
	)
}

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
		} else {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(c.Width)
				gtx.Constraints.Max.X = gtx.Dp(c.Width)
				return w(gtx)
			}))
		}
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
			} else {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(c.Width)
					gtx.Constraints.Max.X = gtx.Dp(c.Width)
					return w(gtx)
				}))
			}
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
