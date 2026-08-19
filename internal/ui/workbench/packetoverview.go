// The Overview tab: what this packet is, in one screen.
//
// It interprets, and Dissection next door evidences - a latitude and a
// longitude are one position here and two fields at two offsets there. The
// two tabs deliberately share nothing: an earlier cut put the frame's
// structure and the raw bytes on both, and the result was two screens that
// read as the same screen twice, which is a fair thing to be asked about.
// Anything that wants a byte offset belongs on the other tab.
package workbench

import (
	"fmt"
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

func (p *packetPanel) overview(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
	rows := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions { return chipRow(t, gtx, pk) },
		func(gtx layout.Context) layout.Dimensions { return payloadPanel(t, gtx, pk) },
	}
	// Only when the chip left a question open; a confirmed scope needs no
	// paragraph.
	if scopeNeedsExplaining(pk.Scope) {
		rows = append(rows, scopeCard(t, pk))
	}
	return comp.List(t, &p.overviewList, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
		return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, rows[i])
	})(gtx)
}

// chipRow is the six facts every packet is asked for, on one row.
//
// Flexed rather than a wrapping grid: six is not a number that varies, and
// letting it wrap put version alone on a second row looking like an
// afterthought.
func chipRow(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
	sc := pk.Scope
	chips := []chip{
		{"from", pk.Origin, fmt.Sprintf("%.2f s", float64(pk.AtMs)/1000), false, ""},
		{"to", destOf(pk), destSub(pk), false, ""},
		{"type", pk.PayloadType, pk.Readable, false, ""},
		// The one chip that carries a claim rather than a reading, so it is
		// the one the eye is sent to first - and the pill says how strong the
		// claim is, because "confirmed" and "unmatched" are different facts.
		{"scope", scopeHeadline(sc), scopeSub(sc), sc.Scoped && sc.Name != "", scopePill(sc)},
		{"hops", fmt.Sprintf("%d", pk.Hops),
			fmt.Sprintf("%d transmissions", pk.Transmissions), false, ""},
		{"version", pk.Version, versionSub(pk), false, ""},
	}
	return equalRow(gtx, t.Sp.XS, len(chips), func(gtx layout.Context, i int) layout.Dimensions {
		return chips[i].layout(t, gtx)
	})
}

// equalRow lays n cells across the full width, all the same width and all as
// tall as the tallest.
//
// Gio's Flex has no stretch on the cross axis, so a row of cards whose text
// wraps to different depths comes out ragged along the bottom - which is what
// the chips did the moment one of them needed a second line. Measuring first
// costs a second layout pass over six small widgets and is the only way to
// know the height before committing to it.
func equalRow(gtx layout.Context, gap unit.Dp, n int, cell func(layout.Context, int) layout.Dimensions) layout.Dimensions {
	if n <= 0 {
		return layout.Dimensions{}
	}
	g := gtx.Dp(gap)
	w := (gtx.Constraints.Max.X - g*(n-1)) / n
	if w < 1 {
		w = 1
	}
	fixedW := gtx
	fixedW.Constraints.Min.X, fixedW.Constraints.Max.X = w, w

	tallest := 0
	for i := 0; i < n; i++ {
		measure := fixedW
		measure.Ops = new(op.Ops)
		if h := cell(measure, i).Size.Y; h > tallest {
			tallest = h
		}
	}

	at := 0
	for i := 0; i < n; i++ {
		cgtx := fixedW
		cgtx.Constraints.Min.Y, cgtx.Constraints.Max.Y = tallest, tallest
		off := op.Offset(image.Pt(at, 0)).Push(gtx.Ops)
		cell(cgtx, i)
		off.Pop()
		at += w + g
	}
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, tallest)}
}

// destOf is who the packet is addressed to.
//
// Not "the last entry in the path" - that is the last node to *relay* it,
// which is a different thing and is not a destination at all on a flood. The
// addressed types carry a destination hash in the payload; everything else is
// broadcast, and a hash is a first byte of a public key, so it is shown as
// the hash it is rather than resolved to a name it only probably belongs to.
func destOf(pk *state.Packet) string {
	for _, f := range pk.PayloadFields {
		if f.Name == "destination hash" {
			return f.Value
		}
	}
	if strings.Contains(pk.PayloadType, "group") {
		return "channel"
	}
	return "broadcast"
}

func destSub(pk *state.Packet) string {
	for _, f := range pk.PayloadFields {
		if f.Name == "destination hash" {
			return "one byte of a public key"
		}
		if f.Name == "channel hash" {
			return "channel " + f.Value
		}
	}
	return pk.RouteType
}

// chip is one labelled fact.
type chip struct {
	label, value, sub string
	// lit marks the chip as carrying a positive answer, drawn in the accent
	// so it reads before the five beside it.
	lit  bool
	pill string
}

func (c chip) layout(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := layout.UniformInset(t.Sp.S).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		valueInk := t.P.Ink
		if c.lit {
			valueInk = t.P.Accent
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, strings.ToUpper(c.label))),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: t.Sp.XXS}.Layout(gtx,
					comp.OneLine(t, t.Sz.Body, valueInk, c.value, true))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if c.sub == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: t.Sp.XXS}.Layout(gtx,
					comp.Text(t, t.Sz.Caption, t.P.Faint, c.sub))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if c.pill == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: t.Sp.XXS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return pillBadge(t, gtx, c.pill, c.lit)
				})
			}),
		)
	})
	call := macro.Stop()
	bg, edge := t.P.Panel, t.P.Rule
	if c.lit {
		bg, edge = theme.Alpha(t.P.Accent, 0.10), t.P.Accent
	}
	comp.RoundRect(gtx, dims.Size, 8, bg)
	comp.Border(gtx, dims.Size, 8, 1, edge)
	call.Add(gtx.Ops)
	return dims
}

// pillBadge is the small outlined word a chip carries when its value needs a
// qualifier - confirmed, or unmatched.
func pillBadge(t *theme.Theme, gtx layout.Context, s string, lit bool) layout.Dimensions {
	ink := t.P.Faint
	if lit {
		ink = t.P.Accent
	}
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{
		Left: t.Sp.XS, Right: t.Sp.XS, Top: t.Sp.XXS, Bottom: t.Sp.XXS,
	}.Layout(gtx, comp.Text(t, t.Sz.Caption, ink, s))
	call := macro.Stop()
	comp.Border(gtx, dims.Size, 999, 1, theme.Alpha(ink, 0.6))
	call.Add(gtx.Ops)
	return dims
}

// scopePill is the qualifier on the scope chip. A scope is only ever
// confirmed against a candidate name, so the word has to say which of the
// three states this is rather than implying a decode.
func scopePill(sc state.PacketScope) string {
	switch {
	case !sc.Scoped, sc.Note != "":
		return ""
	case sc.Name != "":
		return "confirmed"
	case sc.Candidates == 0:
		return "no candidates"
	}
	return "unmatched"
}

// payloadPanel is the readable summary - the fields worth seeing without
// counting offsets, which is what Dissection is for.
func payloadPanel(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
	rows := payloadSummary(pk)
	title := "Payload"
	if n := strings.SplitN(pk.PayloadType, " (", 2); len(n) > 0 && n[0] != "" {
		title = "Payload — " + n[0]
	}
	return titledPanel(t, gtx, title, payloadDetail(pk), func(gtx layout.Context) layout.Dimensions {
		if len(rows) == 0 {
			return comp.Text(t, t.Sz.Caption, t.P.Faint, pk.PayloadNote)(gtx)
		}
		var kids []layout.FlexChild
		for i := range rows {
			r := rows[i]
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				ink := t.P.Ink
				if r.muted {
					ink = t.P.Faint
				}
				return layout.Inset{Bottom: t.Sp.XXS}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							fixed(gtx, 104, comp.Text(t, t.Sz.Caption, t.P.Faint,
								strings.ToUpper(r.label))),
							layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, ink, r.value, true)),
							fixed(gtx, 148, comp.OneLine(t, t.Sz.Caption, t.P.Dim, r.detail, false)),
						)
					})
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	})
}

// payloadDetail is the size and readability, on the panel's own title line.
func payloadDetail(pk *state.Packet) string {
	for _, s := range pk.Spans {
		if strings.HasPrefix(s.Name, "payload") {
			return fmt.Sprintf("%d B, %s", s.Size, s.Detail)
		}
	}
	return ""
}

// sumRow is one line of the payload summary.
type sumRow struct {
	label, value, detail string
	// muted marks a value that is not readable - ciphertext, mostly - so it
	// reads as a fact rather than as a field that failed to load.
	muted bool
}

// payloadSummary turns the dissector's per-field truth into the handful of
// lines a reader wants first.
//
// Presentation only, and only here: the Dissection tab keeps every field
// exactly as it sits in the bytes. Latitude and longitude are one position, a
// flags byte is the node type it encodes - both are two rows of arithmetic in
// the byte view and one fact to a person.
func payloadSummary(pk *state.Packet) []sumRow {
	var out []sumRow
	var lat, lon string
	for _, f := range pk.PayloadFields {
		switch f.Name {
		case "latitude":
			lat = f.Decoded
		case "longitude":
			lon = f.Decoded
		case "flags":
			out = append(out, sumRow{"node type", f.Decoded, "from flags " + f.Value, false})
		case "encrypted":
			out = append(out, sumRow{f.Name, f.Value, f.Description, true})
		default:
			detail := f.Decoded
			if detail == "" {
				detail = fmt.Sprintf("%d B", f.Size)
			}
			out = append(out, sumRow{f.Name, f.Value, detail, false})
		}
	}
	if lat != "" && lon != "" {
		out = append(out, sumRow{"position",
			strings.TrimSuffix(lat, "°") + ", " + strings.TrimSuffix(lon, "°"),
			"decoded from 8 bytes", false})
	}
	return out
}

// titledPanel is a card whose title line carries a right-aligned detail -
// the size, the readability, the hint - which comp.Card has no room for.
func titledPanel(t *theme.Theme, gtx layout.Context, title, detail string, content layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := layout.UniformInset(t.Sp.M).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// A vertical Flex already returns at least Constraints.Min.Y, so a
		// caller pairing two panels sets that and both come out level. Reading
		// Max.Y instead would make a panel laid out on its own fill the whole
		// viewport, which is the shape of the list this sits in.
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: t.Sp.S}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(comp.Text(t, t.Sz.Section, t.P.Ink, title)),
							layout.Flexed(1, comp.Spacer),
							layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, detail)),
						)
					})
			}),
			layout.Rigid(content),
		)
	})
	call := macro.Stop()
	comp.RoundRect(gtx, dims.Size, 8, t.P.Panel)
	comp.Border(gtx, dims.Size, 8, 1, t.P.Rule)
	call.Add(gtx.Ops)
	return dims
}

// want different actions.
func scopeCard(t *theme.Theme, pk *state.Packet) layout.Widget {
	return comp.Card(t, "Region scope", func(gtx layout.Context) layout.Dimensions {
		sc := pk.Scope
		var lines []string
		switch {
		case !sc.Scoped:
			lines = []string{
				"This packet carries no transport code, so it is not scoped to a region.",
				"Only the transport route types carry one.",
			}
		case sc.Note != "":
			lines = []string{
				"The transport codes are 0000 0000, which MeshCore treats as addressed to no region.",
			}
		case sc.Name != "":
			lines = []string{
				"Confirmed as " + sc.Name + ".",
				"The region key is not in the packet - this is the one candidate name whose key " +
					"reproduces the code on the wire, which is confirmation rather than a decode.",
			}
		case sc.Candidates == 0:
			lines = []string{
				"Scoped, but this run holds no region names to check the code against.",
				"Import or infer regions and the same packet will name its scope.",
			}
		default:
			lines = []string{
				fmt.Sprintf("Scoped, but none of the %d region names this run holds produce code %s.",
					sc.Candidates, sc.Code),
				"That means the name is not one we have - not that the packet is unscoped.",
			}
		}
		var kids []layout.FlexChild
		for _, l := range lines {
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: t.Sp.XXS}.Layout(gtx,
					comp.Text(t, t.Sz.Caption, t.P.Dim, l))
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	})
}

// scopeNeedsExplaining is whether the chip alone leaves a question open. A
// confirmed name does not; every other scoped state does.
func scopeNeedsExplaining(sc state.PacketScope) bool {
	return sc.Scoped && sc.Name == ""
}
