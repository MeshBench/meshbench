// The packet view: one transmission, dissected, with everywhere it went.
//
// The old workbench's view, on the new store: CoreScope's dissection plus the
// same packet's fate at every node, because the simulator watched all of them.
package main

import (
	"fmt"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

var packetTabs = []string{"Dissection", "Journey", "Reception ledger", "Where it went"}

type packetPanel struct {
	tabs    [4]comp.Chip
	tab     int
	scroll  widget.List
	ledger  comp.Table
	fates   comp.Table
	journey comp.Table
	close   comp.Button
	built   bool
	do      Do
}

func (p *packetPanel) build() {
	p.ledger.Cols = []comp.Column{
		{Title: "node", Width: 170, Sortable: true},
		{Title: "offered", Width: 64},
		{Title: "RSSI", Width: 70, Right: true, Mono: true, Sortable: true},
		{Title: "SNR", Width: 64, Right: true, Mono: true, Sortable: true},
		{Title: "demod", Width: 60},
		{Title: "CRC", Width: 52},
		{Title: "firmware", Width: 110},
		{Title: "", Width: 50, Menu: true},
	}
	p.fates.Cols = []comp.Column{
		{Title: "t", Width: 64, Right: true, Mono: true, Sortable: true},
		{Title: "node", Width: 170, Sortable: true},
		{Title: "SNR", Width: 64, Right: true, Mono: true},
		{Title: "outcome"},
	}
	p.journey.Cols = []comp.Column{
		{Title: "t", Width: 64, Right: true, Mono: true},
		{Title: "relayed by", Width: 170},
		{Title: "hop", Width: 44, Right: true, Mono: true},
		{Title: "heard by"},
		{Title: "", Width: 50, Menu: true},
	}
	p.close.Label, p.close.Kind = "close", comp.Quiet
	p.built = true
}

func (p *packetPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.built {
		p.build()
	}
	if s == nil || s.Packet == nil {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint,
			"click an event and open its packet view; this panel shows the dissection"))
	}
	pk := s.Packet
	for i := range p.tabs {
		if p.tabs[i].Click.Clicked(gtx) {
			p.tab = i
		}
	}
	if p.close.Click.Clicked(gtx) && p.do != nil {
		p.do("packet.close", nil)
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(comp.SectionTitle(t, fmt.Sprintf("Packet #%d", pk.ID))),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: t.Sp.M}.Layout(gtx,
						comp.Text(t, t.Sz.Caption, t.P.Dim, fmt.Sprintf(
							"from %s at %.2f s  |  heard by %d, missed by %d",
							pk.Origin, float64(pk.AtMs)/1000, pk.Heard, pk.Missed)))
				}),
				layout.Flexed(1, comp.Spacer),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.close.Layout(t, gtx)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if pk.Malformed == "" {
				return layout.Dimensions{}
			}
			return comp.Text(t, t.Sz.Caption, t.P.Warn, "malformed: "+pk.Malformed)(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var kids []layout.FlexChild
			for i := range p.tabs {
				i := i
				label := packetTabs[i]
				if i == 3 {
					label = fmt.Sprintf("%s (%d)", label, len(pk.Fates))
				}
				kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: t.Sp.XS, Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return p.tabs[i].Layout(t, gtx, label, "", p.tab == i, t.P.Accent)
						})
				}))
			}
			return layout.Flex{}.Layout(gtx, kids...)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			switch p.tab {
			case 1:
				return p.drawJourney(t, gtx, pk)
			case 2:
				return p.drawLedger(t, gtx, pk)
			case 3:
				return p.drawFates(t, gtx, pk)
			}
			return p.drawDissection(t, gtx, pk)
		}),
	)
}

// drawDissection is the frame's fields, its path, and the bytes themselves.
func (p *packetPanel) drawDissection(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
	kv := func(k, v string) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(130)
					gtx.Constraints.Max.X = gtx.Dp(130)
					return comp.Text(t, t.Sz.Caption, t.P.Faint, k)(gtx)
				}),
				layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Ink, v)),
			)
		}
	}
	head := func(s string) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.S, Bottom: t.Sp.XS}.Layout(gtx,
				comp.Text(t, t.Sz.Caption, t.P.Dim, strings.ToUpper(s)))
		}
	}
	var rows []layout.Widget
	rows = append(rows, head("header"),
		kv("route type", pk.RouteType),
		kv("payload type", pk.PayloadType),
		kv("version", pk.Version))
	if pk.Transport != "" {
		rows = append(rows, kv("transport codes", pk.Transport))
	}
	rows = append(rows, head(fmt.Sprintf("path - %d hop(s)", len(pk.Path))))
	if len(pk.Path) == 0 {
		rows = append(rows, comp.Text(t, t.Sz.Caption, t.P.Faint,
			"none; this packet has not been relayed"))
	} else {
		rows = append(rows, comp.Text(t, t.Sz.Caption, t.P.Ink,
			strings.Join(pk.Path, "  ->  ")))
	}
	rows = append(rows, head("payload"))
	if len(pk.PayloadFields) == 0 {
		rows = append(rows, comp.Text(t, t.Sz.Caption, t.P.Faint, pk.PayloadNote))
	}
	for _, f := range pk.PayloadFields {
		rows = append(rows, kv(f.Name, f.Value))
	}
	rows = append(rows, head(fmt.Sprintf("raw - %d lines", len(pk.RawLines))))
	for _, l := range pk.RawLines {
		rows = append(rows, comp.Mono(t, t.Sz.Caption, t.P.Dim, l))
	}
	p.scroll.Axis = layout.Vertical
	return comp.List(t, &p.scroll, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
		return rows[i](gtx)
	})(gtx)
}

func yn(b bool, applicable bool) string {
	if !applicable {
		// A dash, not a cross: "did not apply" and "failed" are different
		// facts, and a column that conflates them is why people read logs
		// instead of tables.
		return "-"
	}
	if b {
		return "yes"
	}
	return "no"
}

func (p *packetPanel) drawLedger(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
	reached, decoded := 0, 0
	rows := make([]comp.Row, 0, len(pk.Ledger))
	for _, r := range pk.Ledger {
		if r.Offered {
			reached++
		}
		if r.Demod && r.CRCOK {
			decoded++
		}
		rssi, snr := "-", "-"
		if r.Offered {
			rssi, snr = fmt.Sprintf("%.1f", r.RSSIdBm), fmt.Sprintf("%+.1f", r.SNRdB)
		}
		rows = append(rows, comp.Row{
			Key: r.Node,
			Cells: []string{r.Node, yn(r.Offered, true), rssi, snr,
				yn(r.Demod, r.Offered), yn(r.CRCOK, r.Demod), r.Firmware, "why?"},
		})
	}
	p.ledger.SetRows(rows)
	p.ledger.OnCell = func(key string, col int) {
		if col != 7 || p.do == nil {
			return
		}
		// Why?: select the pair, so the Link and Budget panels answer about
		// exactly this path.
		for _, r := range pk.Ledger {
			if r.Node == key {
				p.do("nodes.select_many", []string{r.From, r.Node})
				p.do("budget.for_selection", nil)
			}
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, fmt.Sprintf(
			"offered to %d nodes . decoded at %d - rows exist for every receiver, "+
				"including nodes whose firmware never knew a frame arrived", reached, decoded))),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.ledger.Layout(t, gtx, nil)
		}),
	)
}

func (p *packetPanel) drawFates(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
	rows := make([]comp.Row, 0, len(pk.Fates))
	for i, f := range pk.Fates {
		c := comp.ClassColour(t, f.Kind)
		snr := ""
		if f.Kind != "tx" {
			snr = fmt.Sprintf("%+.1f", f.SNRdB)
		}
		rows = append(rows, comp.Row{
			Key: fmt.Sprintf("%s/%d", f.Node, i),
			Cells: []string{fmt.Sprintf("%.2f", float64(f.AtMs)/1000),
				f.Node, snr, f.What},
			Tint: [4]uint8{c.R, c.G, c.B, 255},
		})
	}
	p.fates.SetRows(rows)
	return p.fates.Layout(t, gtx, nil)
}

func (p *packetPanel) drawJourney(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
	rows := make([]comp.Row, 0, len(pk.Journey))
	span := 0.0
	if n := len(pk.Journey); n > 0 {
		span = float64(pk.Journey[n-1].AtMs-pk.Journey[0].AtMs) / 1000
	}
	for _, h := range pk.Journey {
		heard := strings.Join(h.Heard, ", ")
		if len(h.Heard) == 0 {
			// A relay nobody heard is the interesting kind: airtime spent for
			// nothing, which is the scoreboard's redundancy figure.
			heard = fmt.Sprintf("nobody (%d could not decode)", h.Missed)
		}
		rows = append(rows, comp.Row{
			Key: fmt.Sprintf("%d", h.PacketID),
			Cells: []string{fmt.Sprintf("%.2f", float64(h.AtMs)/1000),
				h.By, fmt.Sprint(h.Hops), heard, "this"},
		})
	}
	p.journey.SetRows(rows)
	p.journey.OnCell = func(key string, col int) {
		if col != 4 || p.do == nil {
			return
		}
		p.do("packet.open", map[string]any{"id": atof(key)})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, fmt.Sprintf(
			"%d transmissions, reaching %d distinct nodes over %.1f s - one message, "+
				"followed by its payload; the bytes on the air differ at every hop",
			pk.Transmissions, pk.Reached, span))),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.journey.Layout(t, gtx, nil)
		}),
	)
}

// atof reads a numeric row key the way the verbs expect numbers: as float64.
func atof(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
