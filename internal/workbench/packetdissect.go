// The Dissection tab: where every byte of the frame is and what it means.
//
// Overview next door answers "what is this packet" and takes liberties with
// presentation to do it - joining a latitude and a longitude into a position,
// a flags byte into a node type. This file takes none: one row per field, at
// the offset and size it was actually read from, because it is the view
// somebody opens when they do not believe the other one.
package workbench

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// scopeHeadline is the one word the chip leads with.
//
// "unknown" rather than "none" for a code that matched nothing we hold: the
// region key is never in the packet, so a scope can only be confirmed against
// a candidate name, and not holding the name is a different fact from the
// packet having no scope at all.
func scopeHeadline(sc state.PacketScope) string {
	switch {
	case !sc.Scoped:
		return "unscoped"
	case sc.Note != "":
		return "nowhere"
	case sc.Name != "":
		return sc.Name
	}
	return "unknown"
}

func scopeSub(sc state.PacketScope) string {
	switch {
	case !sc.Scoped:
		return "carries no scope code"
	case sc.Note != "":
		return sc.Note
	// The pill beside these carries the verdict - confirmed, unmatched, no
	// candidates - so the sub-line carries only the evidence. Saying
	// "confirmed" twice in two lines of one chip reads as a bug.
	case sc.Name != "", sc.Candidates == 0:
		return "code " + sc.Code
	}
	return fmt.Sprintf("code %s, %d names checked", sc.Code, sc.Candidates)
}

// versionSub says what the payload version decides, which is the width of
// every hash and MAC after it.
func versionSub(pk *state.Packet) string {
	if pk.Version == "0" {
		return "1-byte hashes, 2-byte MAC"
	}
	return "widths undefined in v1.17.0"
}

// dissection is the byte-level view: the frame's shape, every field that
// could be read, and the raw bytes.
func (p *packetPanel) dissection(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
	rows := []layout.Widget{
		p.spansCard(t, pk),
		p.fieldsCard(t, pk),
		p.rawCard(t, pk),
	}
	return comp.List(t, &p.scroll, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
		return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, rows[i])
	})(gtx)
}

// spansCard is the frame's shape - which bytes are header, transport codes,
// path and payload, in order and to scale in bytes.
func (p *packetPanel) spansCard(t *theme.Theme, pk *state.Packet) layout.Widget {
	return comp.Card(t, "Structure", func(gtx layout.Context) layout.Dimensions {
		if len(pk.Spans) == 0 {
			return comp.Text(t, t.Sz.Caption, t.P.Faint, "the frame did not parse")(gtx)
		}
		var kids []layout.FlexChild
		for i := range pk.Spans {
			s := pk.Spans[i]
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: t.Sp.XXS}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							fixed(gtx, 46, comp.Mono(t, t.Sz.Caption, t.P.Faint,
								fmt.Sprintf("%04X", s.Offset))),
							fixed(gtx, 150, comp.OneLine(t, t.Sz.Caption, t.P.Ink, s.Name, false)),
							fixed(gtx, 56, comp.Mono(t, t.Sz.Caption, t.P.Dim,
								fmt.Sprintf("%d B", s.Size))),
							layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Dim, s.Detail, false)),
						)
					})
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	})
}

// fieldsCard is every field the dissector could name, with the offset and
// size it was read from - the columns that turn "here is a value" into "here
// is where that value is in the bytes below".
func (p *packetPanel) fieldsCard(t *theme.Theme, pk *state.Packet) layout.Widget {
	fields := append(append([]state.PacketField{}, pk.PathFields...), pk.PayloadFields...)
	return comp.Card(t, fmt.Sprintf("Fields - %d read", len(fields)),
		func(gtx layout.Context) layout.Dimensions {
			if len(fields) == 0 {
				return comp.Text(t, t.Sz.Caption, t.P.Faint, pk.PayloadNote)(gtx)
			}
			var kids []layout.FlexChild
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: t.Sp.XXS}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{}.Layout(gtx,
							fixed(gtx, 46, comp.Text(t, t.Sz.Caption, t.P.Faint, "at")),
							fixed(gtx, 130, comp.Text(t, t.Sz.Caption, t.P.Faint, "field")),
							fixed(gtx, 44, comp.Text(t, t.Sz.Caption, t.P.Faint, "size")),
							fixed(gtx, 190, comp.Text(t, t.Sz.Caption, t.P.Faint, "value")),
							layout.Flexed(1, comp.Text(t, t.Sz.Caption, t.P.Faint, "meaning")),
							fixed(gtx, 56, comp.Spacer),
						)
					})
			}))
			for i := range fields {
				f := fields[i]
				key := fmt.Sprintf("field:%d", i)
				ck, ok := p.whyBtns[key]
				if !ok {
					ck = &widget.Clickable{}
					p.whyBtns[key] = ck
				}
				if ck.Clicked(gtx) {
					copyText(gtx, f.Value)
				}
				kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return fieldRow(t, gtx, f, ck)
				}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
		})
}

// fieldRow is one dissected field. The decoded reading sits beside the raw
// value rather than replacing it: the reading is the answer, and the raw
// value is what lets somebody check it against the hex.
func fieldRow(t *theme.Theme, gtx layout.Context, f state.PacketField, ck *widget.Clickable) layout.Dimensions {
	meaning := f.Description
	if f.Decoded != "" {
		meaning = f.Decoded
		if f.Description != "" {
			meaning += " - " + f.Description
		}
	}
	// An unread field is one the dissector deliberately stopped at, and it is
	// coloured as a fact rather than a failure.
	valueInk := t.P.Ink
	if f.Name == "encrypted" {
		valueInk = t.P.Faint
	}
	return layout.Inset{Top: t.Sp.XXS, Bottom: t.Sp.XXS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				fixed(gtx, 46, comp.Mono(t, t.Sz.Caption, t.P.Faint,
					fmt.Sprintf("%04X", f.Offset))),
				fixed(gtx, 130, comp.OneLine(t, t.Sz.Caption, t.P.Dim, f.Name, false)),
				fixed(gtx, 44, comp.Mono(t, t.Sz.Caption, t.P.Faint,
					fmt.Sprintf("%d", f.Size))),
				fixed(gtx, 190, comp.OneLine(t, t.Sz.Caption, valueInk, f.Value, true)),
				layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Dim, meaning, false)),
				fixed(gtx, 56, func(gtx layout.Context) layout.Dimensions {
					return borderedAction(t, gtx, ck, "copy", t.P.Rule, t.P.Dim)
				}),
			)
		})
}

// fixed pins a cell to a width in dp, so the columns of a hand-built table
// line up with their own heading.
func fixed(gtx layout.Context, w int, wgt layout.Widget) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		px := gtx.Dp(unitDp(w))
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = px, px
		d := wgt(gtx)
		d.Size.X = px
		return d
	})
}
