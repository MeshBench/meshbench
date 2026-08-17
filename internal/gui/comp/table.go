package comp

import (
	"strings"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"

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

	// widths are per-column overrides in dp: what the operator has dragged a
	// column to, or what the content asked for. Empty means the declared
	// width, so a table nobody has touched looks as its panel declared it.
	widths []unit.Dp
	// grips are the drag handles between headers, pooled by column.
	grips []Splitter
	// fitted records that the content has been measured once. Re-measuring on
	// every SetRows would fight an operator mid-drag.
	fitted bool

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
	for len(tb.grips) < len(tb.Cols) {
		tb.grips = append(tb.grips, Splitter{Vertical: true})
	}
	for len(tb.widths) < len(tb.Cols) {
		tb.widths = append(tb.widths, 0)
	}
	tb.autoFit(t)
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

// widthOf is what a column is actually drawn at: what it has been dragged or
// fitted to, else what the panel declared. Zero still means "take the rest".
func (tb *Table) widthOf(i int) unit.Dp {
	if i < len(tb.widths) && tb.widths[i] > 0 {
		return tb.widths[i]
	}
	return tb.Cols[i].Width
}

// autoFit sizes every fixed column to its widest cell, once.
//
// The alternative is a number somebody chose when the table was written, which
// is right for the data they had in front of them: the Runs table clipped
// "1.17.0 · agc.reset.interval 0" to "1.17.0 · agc.re" because 200dp was
// generous for a version string and nothing else.
//
// Measured in characters rather than shaped, deliberately. Shaping every cell
// of a twenty-thousand row ledger to pick a column width costs more than the
// clipping does, and an over-estimate only wastes space while an under-estimate
// hides data.
func (tb *Table) autoFit(t *theme.Theme) {
	if tb.fitted || len(tb.rows2()) == 0 {
		return
	}
	tb.fitted = true
	for len(tb.widths) < len(tb.Cols) {
		tb.widths = append(tb.widths, 0)
	}
	for i := range tb.Cols {
		if tb.Cols[i].Width == 0 {
			continue // the flexed column takes what is left
		}
		widest := len(tb.Cols[i].Title)
		for _, r := range tb.rows2() {
			if i < len(r.Cells) && len(r.Cells[i]) > widest {
				widest = len(r.Cells[i])
			}
		}
		// Per character, plus the cell's own padding. Mono is near enough
		// exact; proportional over-estimates, which errs towards space.
		per := t.Sz.Body * 6 / 10
		want := unit.Dp(float32(widest)*float32(per)/10) + 2*t.Sp.S
		switch {
		case want < 48:
			want = 48
		case want > 420:
			want = 420 // past this a column is a paragraph; drag it wider
		}
		if want > tb.Cols[i].Width {
			tb.widths[i] = want
		}
	}
}

// rows2 is every row the table holds, filtered or not.
func (tb *Table) rows2() []Row { return tb.shown }
