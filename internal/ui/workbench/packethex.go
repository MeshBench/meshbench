// The hex view, shared by both tabs that show one.
//
// Overview picks out the payload; Dissection picks out whichever field is
// selected. Same renderer either way, because they are the same question -
// where in these bytes is the thing I am reading - and two hex dumps that
// drift apart would be two answers to it.
package workbench

import (
	"fmt"
	"strings"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// hexPanel is the frame's bytes with one range picked out - the payload on
// Overview, the selected field on Dissection.
func hexPanel(t *theme.Theme, gtx layout.Context, pk *state.Packet, from, to int, note string) layout.Dimensions {
	return titledPanel(t, gtx, fmt.Sprintf("Raw — %d bytes", len(pk.Raw)),
		note, func(gtx layout.Context) layout.Dimensions {
			var lines []layout.FlexChild
			for off := 0; off < len(pk.Raw); off += 16 {
				end := off + 16
				if end > len(pk.Raw) {
					end = len(pk.Raw)
				}
				row := pk.Raw[off:end]
				at := off
				lines = append(lines, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return hexLine(t, gtx, at, row, from, to)
				}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, lines...)
		})
}

// hexLine is one 16-byte row: the offset, then the bytes in up to three runs
// - before the picked-out range, inside it, and after it - then the printable
// ASCII. Three runs rather than one span per byte because a selected field is
// contiguous, and sixteen separate labels a line is a lot of layout to do
// sixty times a second for a colour change.
func hexLine(t *theme.Theme, gtx layout.Context, at int, row []byte, from, to int) layout.Dimensions {
	var pre, hit, post strings.Builder
	var ascii strings.Builder
	for i, b := range row {
		switch off := at + i; {
		case off < from:
			fmt.Fprintf(&pre, "%02X ", b)
		case off < to:
			fmt.Fprintf(&hit, "%02X ", b)
		default:
			fmt.Fprintf(&post, "%02X ", b)
		}
		if b >= 0x20 && b < 0x7F {
			ascii.WriteByte(b)
		} else {
			ascii.WriteByte('.')
		}
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		fixed(gtx, 46, comp.Mono(t, t.Sz.Caption, t.P.Faint, fmt.Sprintf("%04X", at))),
		layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Dim, pre.String())),
		layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Accent, hit.String())),
		layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Dim, post.String())),
		layout.Flexed(1, comp.Spacer),
		layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Faint, ascii.String())),
	)
}

// scopeCard explains a scope the chip cannot say in three words.
//
// Only shown when there is something to explain. A confirmed scope needs no
// paragraph - the chip names it and the pill says it was confirmed. The cases
// that do need one are the ambiguous ones, where an operator who cannot see
// what was checked would read "not scoped to anything" and "scoped to
// something we could not name" as the same result. They are not, and they
