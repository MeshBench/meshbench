// The "why?" modal: every time one node was offered a packet across its
// whole journey, and what became of each attempt - drawn over the packet
// panel rather than sending the operator to another one, because the
// question is about this packet, not about the link in general.
package workbench

import (
	"fmt"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// whyModal draws the overlay when a "why?" button has been clicked, and
// nothing when one has not.
func (p *packetPanel) whyModal(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
	if p.whyOpen == "" {
		return layout.Dimensions{}
	}
	var rows []state.PacketReception
	for _, r := range pk.LedgerFull {
		if r.Node == p.whyOpen {
			rows = append(rows, r)
		}
	}
	if len(rows) == 0 {
		// The view moved on without this node - a rebuild while the packet
		// is still propagating can do that - so there is nothing left to
		// answer for.
		p.whyOpen = ""
		return layout.Dimensions{}
	}

	// Escape closes it, the same as every other modal in this application.
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
			p.whyOpen = ""
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}
	}
	if p.whyClose.Click.Clicked(gtx) {
		p.whyOpen = ""
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}

	// Dim what is behind it, so it reads as one answer rather than a panel
	// that has appeared in the middle of the window.
	comp.FillRect(gtx, gtx.Constraints.Max, theme.Alpha(t.P.Ground, 0.82))

	// Modal means the frame underneath does not get the clicks.
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, p)
	for {
		if _, ok := gtx.Event(pointer.Filter{
			Target: p, Kinds: pointer.Press | pointer.Release,
		}); !ok {
			break
		}
	}

	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = gtx.Dp(460)
		gtx.Constraints.Min.X = gtx.Dp(460)
		if gtx.Constraints.Max.Y > gtx.Dp(420) {
			gtx.Constraints.Max.Y = gtx.Dp(420)
		}
		macro := op.Record(gtx.Ops)
		dims := layout.UniformInset(t.Sp.M).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, comp.Text(t, t.Sz.Title, t.P.Ink, p.whyOpen)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return p.whyClose.Layout(t, gtx)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: t.Sp.XXS}.Layout),
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, whySummary(len(rows)))),
				layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
				layout.Rigid(comp.HRule(t)),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return comp.List(t, &p.whyList, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
						return whyRow(t, gtx, rows[i])
					})(gtx)
				}),
			)
		})
		call := macro.Stop()
		comp.RoundRect(gtx, dims.Size, 6, t.P.Panel)
		comp.Border(gtx, dims.Size, 6, 1, t.P.Rule)
		call.Add(gtx.Ops)
		return dims
	})
}

// whySummary says how many attempts there were to answer for, so the count
// in the list below is never the first thing that has to be counted by eye.
func whySummary(n int) string {
	if n == 1 {
		return "offered this packet once across the whole journey"
	}
	return fmt.Sprintf("offered this packet %d times across the whole journey", n)
}

// whyRow is one attempt: which hop, who sent it, and what became of it.
func whyRow(t *theme.Theme, gtx layout.Context, r state.PacketReception) layout.Dimensions {
	status, ink := receptionStatus(t, r)
	return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
							fmt.Sprintf("hop %d, from %s", r.Hop, r.From))),
						layout.Flexed(1, comp.Spacer),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if !r.Offered {
								return layout.Dimensions{}
							}
							return comp.Mono(t, t.Sz.Caption, t.P.Faint, fmt.Sprintf(
								"%.1f dBm, %+.1f dB SNR", r.RSSIdBm, r.SNRdB))(gtx)
						}),
					)
				}),
				layout.Rigid(comp.Text(t, t.Sz.Caption, ink, status)),
			)
		})
}

// receptionStatus is the one line that answers "why" for a single attempt:
// the engine's own words when it has them, and a plain fact when it does not
// (out of range never had a reason to give, and a clean decode is not a
// failure looking for an explanation).
func receptionStatus(t *theme.Theme, r state.PacketReception) (string, colorNRGBA) {
	switch {
	case !r.Offered:
		return "out of range - nothing measurable arrived", t.P.Faint
	case r.Why != "":
		return r.Why, t.P.Bad
	case r.Demod && r.CRCOK:
		return r.Firmware, t.P.Good
	default:
		return "offered, but not decoded", t.P.Warn
	}
}
