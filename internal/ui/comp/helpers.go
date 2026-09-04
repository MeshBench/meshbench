// The three helpers every view reached for and none of them owned.
//
// All three lived in panel files - two in the packet inspector and one in
// firmwarepanel.go - and were used from across the interface. A helper that
// half the package calls does not belong in whichever panel happened to need it
// first: the next reader looks for it where it is used and finds it filed under
// something unrelated, and moving that panel takes the helper with it.
//
// They are here because comp is where the widgets every view is built from
// live, and these are widgets in everything but name.
package comp

import (
	"image/color"
	"io"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// Fixed lays a widget out at exactly w device-independent pixels wide.
//
// Both constraints, not just the maximum: a table cell that is allowed to
// shrink below its column is a table whose rows do not line up, and the
// misalignment appears a dozen rows down where nobody connects it to the cell
// that gave way.
func Fixed(gtx layout.Context, w int, wgt layout.Widget) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		px := gtx.Dp(unit.Dp(w))
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = px, px
		// The returned width is forced too, not only the constraints. A
		// widget is entitled to report less than it was given, and a cell
		// that does slides every column after it off its own heading - which
		// shows up a dozen rows down, nowhere near the cell that gave way.
		d := wgt(gtx)
		d.Size.X = px
		return d
	})
}

// CopyText puts s on the system clipboard.
//
// Through gtx.Execute rather than a clipboard handle of its own, because Gio
// routes a write through the frame that asked for it: a copy issued outside one
// reaches a clipboard nobody is holding.
func CopyText(gtx layout.Context, s string) {
	gtx.Execute(clipboard.WriteCmd{Type: "application/text",
		Data: io.NopCloser(strings.NewReader(s))})
}

// BorderedAction is a labelled control drawn as an outline rather than a fill,
// for an action offered beside content it must not compete with.
//
// The ink brightens on hover instead of the ground filling, so a row of these
// does not flash as a pointer crosses it.
func BorderedAction(t *theme.Theme, gtx layout.Context, ck *widget.Clickable,
	label string, line, ink color.NRGBA) layout.Dimensions {
	return ck.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if ck.Hovered() {
			ink = t.P.Ink
		}
		// Recorded and replayed, so the ground and the border are painted
		// under the label rather than over it.
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{
			Top: t.Sp.XS, Bottom: t.Sp.XS, Left: t.Sp.S, Right: t.Sp.S,
		}.Layout(gtx, Text(t, t.Sz.Caption, ink, label))
		call := macro.Stop()
		RoundRect(gtx, dims.Size, 5, theme.Alpha(t.P.Sunk, 0.6))
		Border(gtx, dims.Size, 5, 1, line)
		call.Add(gtx.Ops)
		return dims
	})
}
