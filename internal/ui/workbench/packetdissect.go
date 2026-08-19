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
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
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
	fields := dissectedFields(pk)
	// Clicks are taken here, before anything reads the selection. Handling
	// them inside the cards below meant the hex was drawn from the previous
	// frame's answer and a click appeared to do nothing until the next one.
	p.takeSelectionClicks(gtx, pk, fields)
	from, to := p.selectedBytes(pk, fields)
	rows := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions { return structurePanel(t, gtx, pk, p) },
		p.fieldsCard(t, pk),
		func(gtx layout.Context) layout.Dimensions {
			return hexPanel(t, gtx, pk, from, to, p.selectionNote(pk, fields))
		},
	}
	return comp.List(t, &p.scroll, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
		return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, rows[i])
	})(gtx)
}

// takeSelectionClicks reads this frame's clicks on both tables. A field and a
// span are the same kind of answer - a range of bytes - so they share one
// selection and each clears the other.
func (p *packetPanel) takeSelectionClicks(gtx layout.Context, pk *state.Packet, fields []state.PacketField) {
	for i := range fields {
		ck := p.click(fmt.Sprintf("fieldrow:%d", i))
		if ck.Clicked(gtx) {
			if p.selField == i && p.selSpan < 0 {
				p.selField = -1
			} else {
				p.selField, p.selSpan = i, -1
			}
		}
	}
	for i := range pk.Spans {
		ck := p.click(fmt.Sprintf("spanrow:%d", i))
		if ck.Clicked(gtx) {
			if p.selSpan == i {
				p.selSpan = -1
			} else {
				p.selSpan, p.selField = i, -1
			}
		}
	}
}

// click is one named clickable, made on first use.
func (p *packetPanel) click(key string) *widget.Clickable {
	ck, ok := p.whyBtns[key]
	if !ok {
		ck = &widget.Clickable{}
		p.whyBtns[key] = ck
	}
	return ck
}

// selectedBytes is the range the hex picks out: a chosen field, a chosen
// structural span, or - with nothing chosen - the payload, so the view always
// says something rather than opening blank.
func (p *packetPanel) selectedBytes(pk *state.Packet, fields []state.PacketField) (int, int) {
	if p.selField >= 0 && p.selField < len(fields) {
		f := fields[p.selField]
		return f.Offset, f.Offset + f.Size
	}
	if p.selSpan >= 0 && p.selSpan < len(pk.Spans) {
		s := pk.Spans[p.selSpan]
		return s.Offset, s.Offset + s.Size
	}
	for _, s := range pk.Spans {
		if strings.HasPrefix(s.Name, "payload") {
			return s.Offset, s.Offset + s.Size
		}
	}
	return 0, 0
}

// dissectedFields is every named field in one order - the order the table
// lists them and the order selField indexes into. Both read it, so a row
// cannot come to mean a different field to the table than to the hex.
func dissectedFields(pk *state.Packet) []state.PacketField {
	return append(append([]state.PacketField{}, pk.PathFields...), pk.PayloadFields...)
}

// selectionNote says what is picked out, and how to pick something else.
func (p *packetPanel) selectionNote(pk *state.Packet, fields []state.PacketField) string {
	if p.selField >= 0 && p.selField < len(fields) {
		f := fields[p.selField]
		return fmt.Sprintf("%s — %d bytes at %04X", f.Name, f.Size, f.Offset)
	}
	if p.selSpan >= 0 && p.selSpan < len(pk.Spans) {
		s := pk.Spans[p.selSpan]
		return fmt.Sprintf("%s — %d bytes at %04X", s.Name, s.Size, s.Offset)
	}
	return "the payload — click any row above to pick out its bytes"
}

func (p *packetPanel) fieldsCard(t *theme.Theme, pk *state.Packet) layout.Widget {
	fields := dissectedFields(pk)
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
				copyKey := fmt.Sprintf("field:%d", i)
				ck, ok := p.whyBtns[copyKey]
				if !ok {
					ck = &widget.Clickable{}
					p.whyBtns[copyKey] = ck
				}
				if ck.Clicked(gtx) {
					copyText(gtx, f.Value)
				}
				rk := p.click(fmt.Sprintf("fieldrow:%d", i))
				sel := p.selField == i
				kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return rk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return fieldRow(t, gtx, f, ck, sel)
					})
				}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
		})
}

// fieldRow is one dissected field. The decoded reading sits beside the raw
// value rather than replacing it: the reading is the answer, and the raw
// value is what lets somebody check it against the hex.
func fieldRow(t *theme.Theme, gtx layout.Context, f state.PacketField, ck *widget.Clickable, sel bool) layout.Dimensions {
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
	if sel {
		valueInk = t.P.Accent
	}
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Top: t.Sp.XXS, Bottom: t.Sp.XXS}.Layout(gtx,
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
	call := macro.Stop()
	if sel {
		comp.RoundRect(gtx, dims.Size, 4, theme.Alpha(t.P.Accent, 0.12))
	}
	call.Add(gtx.Ops)
	return dims
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

// structurePanel is the frame's shape: four spans that always exist, each
// with what it cost in bytes, and the payload picked out as the one the rest
// of this tab is about.
func structurePanel(t *theme.Theme, gtx layout.Context, pk *state.Packet, p *packetPanel) layout.Dimensions {
	return titledPanel(t, gtx, "Structure", "", func(gtx layout.Context) layout.Dimensions {
		if len(pk.Spans) == 0 {
			return comp.Text(t, t.Sz.Caption, t.P.Faint, "the frame did not parse")(gtx)
		}
		var kids []layout.FlexChild
		for i := range pk.Spans {
			s := pk.Spans[i]
			// Lit when chosen, or - with nothing chosen - the payload, which
			// is what the hex picks out by default.
			lit := p.selSpan == i ||
				(p.selSpan < 0 && p.selField < 0 && strings.HasPrefix(s.Name, "payload"))
			ck := p.click(fmt.Sprintf("spanrow:%d", i))
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: t.Sp.XXS}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return ck.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return spanRow(t, gtx, s, lit)
						})
					})
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	})
}

// spanRow is one structural region, boxed so the four read as a stack of
// parts rather than as a list of words.
func spanRow(t *theme.Theme, gtx layout.Context, s state.PacketSpan, lit bool) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{
		Left: t.Sp.S, Right: t.Sp.S, Top: t.Sp.XS, Bottom: t.Sp.XS,
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		ink := t.P.Dim
		if lit {
			ink = t.P.Accent
		}
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			fixed(gtx, 46, comp.Mono(t, t.Sz.Caption, t.P.Faint,
				fmt.Sprintf("%04X", s.Offset))),
			fixed(gtx, 170, comp.OneLine(t, t.Sz.Caption, ink, s.Name, false)),
			layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Dim, s.Detail, false)),
			fixed(gtx, 64, comp.Mono(t, t.Sz.Caption, t.P.Faint, spanSize(s))),
		)
	})
	call := macro.Stop()
	bg, edge := t.P.Sunk, t.P.Rule
	if lit {
		bg, edge = theme.Alpha(t.P.Accent, 0.10), t.P.Accent
	}
	comp.RoundRect(gtx, dims.Size, 6, bg)
	comp.Border(gtx, dims.Size, 6, 1, edge)
	call.Add(gtx.Ops)
	return dims
}

// spanSize prefers hops to bytes for the path, because hops is what the path
// is counted in everywhere else in this window.
func spanSize(s state.PacketSpan) string {
	if s.Name == "path" || s.Name == "path length" {
		if n := strings.Fields(s.Detail); len(n) > 1 && n[1] == "hops" {
			return n[0] + " hops"
		}
	}
	return fmt.Sprintf("%d B", s.Size)
}
