// The two tabs that answer "what happened at every node": the reception
// ledger, which is the radio-level truth per receiver, and the fates table,
// which is one transmission's outcome everywhere.
//
// Split from packetpanel.go on size. The seam is real - these two read the
// run's own record of what each node did, where the tabs beside them read the
// bytes of one frame.
package packetview

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// ledger: the radio-level truth per receiver, yes and no in their colours,
// with the way into the link that explains each row.
func (p *Panel) ledger(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
	cols := []struct {
		label string
		width int
	}{
		{"Node", 180}, {"Offered", 70}, {"RSSI", 76}, {"SNR", 70},
		{"Demod", 64}, {"CRC", 56}, {"Firmware", 110}, {"", 60},
	}
	cell := func(w int, wgt layout.Widget) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			px := gtx.Dp(unit.Dp(w))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = px, px
			d := wgt(gtx)
			d.Size.X = px
			return d
		})
	}
	ynText := func(b, applicable bool) layout.Widget {
		if !applicable {
			// A dash, not a cross: "did not apply" and "failed" are different
			// facts, and conflating them is why people read logs instead.
			return comp.Text(t, t.Sz.Caption, t.P.Faint, "-")
		}
		if b {
			return comp.Text(t, t.Sz.Caption, t.P.Good, "yes")
		}
		return comp.Text(t, t.Sz.Caption, t.P.Bad, "no")
	}
	reached, decoded := 0, 0
	for _, r := range pk.Ledger {
		if r.Offered {
			reached++
		}
		if r.Demod && r.CRCOK {
			decoded++
		}
	}
	var heads []layout.FlexChild
	for _, c := range cols {
		c := c
		heads = append(heads, cell(c.width, comp.Text(t, t.Sz.Caption, t.P.Faint, c.label)))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, fmt.Sprintf(
			"offered to %d . decoded at %d - rows exist for every receiver, including "+
				"nodes whose firmware never knew a frame arrived", reached, decoded))),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.S, Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{}.Layout(gtx, heads...)
				})
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return comp.List(t, &p.lList, len(pk.Ledger), func(gtx layout.Context, i int) layout.Dimensions {
				r := pk.Ledger[i]
				// Keyed by row index, not by node: a node offered on more
				// than one hop now has more than one row, and two rows
				// sharing one Clickable would corrupt each other's click
				// state - only whichever laid out last would ever answer.
				key := fmt.Sprintf("ledger:%d", i)
				ck, ok := p.whyBtns[key]
				if !ok {
					ck = &widget.Clickable{}
					p.whyBtns[key] = ck
				}
				if ck.Clicked(gtx) {
					// Why?: every time this node was offered the packet
					// across its whole journey, and why each attempt did or
					// did not land - not just this one row's hop.
					p.whyOpen = r.Node
				}
				rssi, snr := "-", "-"
				if r.Offered {
					rssi, snr = fmt.Sprintf("%.1f", r.RSSIdBm), fmt.Sprintf("%+.1f", r.SNRdB)
				}
				fwInk := t.P.Faint
				switch r.Firmware {
				case "accepted":
					fwInk = t.P.Good
				case "dropped":
					fwInk = t.P.Warn
				}
				return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							cell(cols[0].width, comp.OneLine(t, t.Sz.Caption, t.P.Ink, r.Node, false)),
							cell(cols[1].width, ynText(r.Offered, true)),
							cell(cols[2].width, comp.Mono(t, t.Sz.Caption, t.P.Dim, rssi)),
							cell(cols[3].width, comp.Mono(t, t.Sz.Caption, t.P.Dim, snr)),
							cell(cols[4].width, ynText(r.Demod, r.Offered)),
							cell(cols[5].width, ynText(r.CRCOK, r.Demod)),
							cell(cols[6].width, comp.Text(t, t.Sz.Caption, fwInk, r.Firmware)),
							cell(cols[7].width, func(gtx layout.Context) layout.Dimensions {
								return comp.BorderedAction(t, gtx, ck, "why?", t.P.Rule, t.P.Dim)
							}),
						)
					})
			})(gtx)
		}),
	)
}

// whereItWent: every node's outcome for this one transmission.
func (p *Panel) whereItWent(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
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
