// The packet view, to Alex's mocks: one transmission, dissected, with
// everywhere it went - and the same packet's fate at every node, which no
// real capture can produce because no observer is everywhere.
package workbench

import (
	"fmt"
	"io"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

var packetTabs = []string{"Dissection", "Journey", "Reception ledger", "Where it went"}

// packetOpenOnTab is which tab a freshly built Packet panel shows, set by
// -packet-tab.
//
// The same reason the node window has one: a tab that can only be reached by
// clicking is a tab no screenshot can capture and no script can drive, and the
// propagation graph lives on the second of these.
var packetOpenOnTab int

type packetPanel struct {
	tabs    [4]widget.Clickable
	tab     int
	scroll  widget.List
	jList   widget.List
	lList   widget.List
	fates   comp.Table
	whyBtns map[string]*widget.Clickable
	ascii   bool
	hexBtn  comp.Chip
	ascBtn  comp.Chip
	// graphBtn folds the propagation picture away. Docked in a third of a
	// window there is not room for both it and the table, and which one you
	// want depends on whether you are asking "what shape" or "what exactly".
	graphBtn comp.Chip
	noGraph  bool
	copyBtn  comp.Button
	prev     comp.Button
	next     comp.Button
	close    comp.Button
	built    bool
	do       Do
}

func (p *packetPanel) build() {
	p.fates.Cols = []comp.Column{
		{Title: "t", Width: 64, Right: true, Mono: true, Sortable: true},
		{Title: "node", Width: 170, Sortable: true},
		{Title: "SNR", Width: 64, Right: true, Mono: true},
		{Title: "outcome"},
	}
	p.copyBtn.Label, p.copyBtn.Kind = "copy packet", comp.Secondary
	p.prev.Label, p.prev.Kind = "prev", comp.Secondary
	p.next.Label, p.next.Kind = "next", comp.Secondary
	p.close.Label, p.close.Kind = "close", comp.Primary
	p.whyBtns = map[string]*widget.Clickable{}
	p.scroll.Axis, p.jList.Axis, p.lList.Axis = layout.Vertical, layout.Vertical, layout.Vertical
	if packetOpenOnTab > 0 && packetOpenOnTab < len(packetTabs) {
		p.tab = packetOpenOnTab
	}
	p.built = true
}

// copyText puts a string on the system clipboard.
func copyText(gtx layout.Context, s string) {
	gtx.Execute(clipboard.WriteCmd{Type: "application/text",
		Data: io.NopCloser(strings.NewReader(s))})
}

func (p *packetPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.built {
		p.build()
	}
	if s == nil || s.Packet == nil {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint,
			"click an event to open its packet here"))
	}
	pk := s.Packet
	for i := range p.tabs {
		if p.tabs[i].Clicked(gtx) {
			p.tab = i
		}
	}
	if p.close.Click.Clicked(gtx) && p.do != nil {
		p.do("packet.close", nil)
	}
	if p.prev.Click.Clicked(gtx) && p.do != nil {
		p.do("packet.open", map[string]any{"id": float64(pk.ID - 1), "seek": -1.0})
	}
	if p.next.Click.Clicked(gtx) && p.do != nil {
		p.do("packet.open", map[string]any{"id": float64(pk.ID + 1), "seek": 1.0})
	}
	if p.copyBtn.Click.Clicked(gtx) {
		copyText(gtx, packetText(pk))
	}
	if p.graphBtn.Click.Clicked(gtx) {
		p.noGraph = !p.noGraph
	}
	if p.hexBtn.Click.Clicked(gtx) {
		p.ascii = false
	}
	if p.ascBtn.Click.Clicked(gtx) {
		p.ascii = true
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.header(t, gtx, pk)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.tabStrip(t, gtx, pk)
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					switch p.tab {
					case 1:
						return p.journey(t, gtx, pk)
					case 2:
						return p.ledger(t, gtx, pk)
					case 3:
						return p.whereItWent(t, gtx, pk)
					}
					return p.dissection(t, gtx, pk)
				})
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.footer(t, gtx, pk)
		}),
	)
}

// header: the packet, where it came from, and how it fared - green and red
// carrying the two numbers that matter.
func (p *packetPanel) header(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
	return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Title, t.P.Ink, fmt.Sprintf("Packet #%d", pk.ID))),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
						fmt.Sprintf("from %s at %.2f s  |  heard by ", pk.Origin,
							float64(pk.AtMs)/1000))),
					layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Good,
						fmt.Sprintf("%d", pk.Heard))),
					layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, ", missed by ")),
					layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Bad,
						fmt.Sprintf("%d", pk.Missed))),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if pk.Malformed == "" {
							return layout.Dimensions{}
						}
						return comp.Text(t, t.Sz.Caption, t.P.Warn,
							"   malformed: "+pk.Malformed)(gtx)
					}),
				)
			}),
		)
	})
}

// tabStrip is the mock's underlined tabs.
func (p *packetPanel) tabStrip(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
	var kids []layout.FlexChild
	for i := range p.tabs {
		i := i
		label := packetTabs[i]
		if i == 3 {
			label = fmt.Sprintf("%s (%d)", label, len(pk.Fates))
		}
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.tabs[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				ink := t.P.Dim
				if p.tab == i || p.tabs[i].Hovered() {
					ink = t.P.Ink
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Right: t.Sp.L, Top: t.Sp.XS,
							Bottom: t.Sp.XS}.Layout(gtx,
							comp.Text(t, t.Sz.Body, ink, label))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if p.tab != i {
							return layout.Dimensions{Size: imagePtXY(0, gtx.Dp(2))}
						}
						w := gtx.Constraints.Min.X
						return comp.FillRect(gtx, imagePtXY(w, gtx.Dp(2)), t.P.Accent)
					}),
				)
			})
		}))
	}
	return layout.Flex{}.Layout(gtx, kids...)
}

// statBox is the mock's labelled cell: an uppercase caption over a mono value,
// with an optional second line.
func statBox(t *theme.Theme, label, value, sub string) layout.Widget {
	return comp.Card(t, "", func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, strings.ToUpper(label))),
			layout.Rigid(comp.Mono(t, t.Sz.Body, t.P.Ink, value)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if sub == "" {
					return layout.Dimensions{}
				}
				return comp.Text(t, t.Sz.Caption, t.P.Faint, sub)(gtx)
			}),
		)
	})
}

// dissection: the header as cards, the payload with its copy buttons, and the
// raw bytes with a hex/ascii toggle.
func (p *packetPanel) dissection(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
	dest, via := "broadcast", ""
	if n := len(pk.Path); n > 0 {
		dest = pk.Path[n-1]
		via = "via " + strings.Join(pk.Path, " -> ")
	}
	size := ""
	if n := len(pk.RawLines); n > 0 {
		// The last line's offset plus its bytes; simpler to carry the count.
		size = fmt.Sprintf("%d lines", n)
	}
	_ = size
	rows := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return comp.CellGrid(t, gtx, 170, []layout.Widget{
				statBox(t, "route", pk.Origin, fmt.Sprintf("%.2f s", float64(pk.AtMs)/1000)),
				statBox(t, "destination", dest, via),
				statBox(t, "route type", pk.RouteType, "payload  "+pk.PayloadType),
				statBox(t, "version", pk.Version, transportSub(pk)),
				statBox(t, "hops", fmt.Sprintf("%d", len(pk.Path)),
					fmt.Sprintf("%d transmissions of the message", pk.Transmissions)),
			})
		},
		p.payloadCard(t, pk),
		p.rawCard(t, pk),
	}
	return comp.List(t, &p.scroll, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
		return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, rows[i])
	})(gtx)
}

func transportSub(pk *state.Packet) string {
	if pk.Transport == "" {
		return ""
	}
	return "transport  " + pk.Transport
}

// payloadCard: what the payload carries in clear, each long value with its
// own copy button.
func (p *packetPanel) payloadCard(t *theme.Theme, pk *state.Packet) layout.Widget {
	return comp.Card(t, "Payload", func(gtx layout.Context) layout.Dimensions {
		if len(pk.PayloadFields) == 0 {
			return comp.Text(t, t.Sz.Caption, t.P.Faint, pk.PayloadNote)(gtx)
		}
		var kids []layout.FlexChild
		for i := range pk.PayloadFields {
			f := pk.PayloadFields[i]
			key := "pay/" + f.Name
			ck, ok := p.whyBtns[key]
			if !ok {
				ck = &widget.Clickable{}
				p.whyBtns[key] = ck
			}
			if ck.Clicked(gtx) {
				copyText(gtx, f.Value)
			}
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								w := gtx.Dp(110)
								gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
								d := comp.Text(t, t.Sz.Caption, t.P.Faint,
									strings.ToUpper(f.Name))(gtx)
								d.Size.X = w
								return d
							}),
							layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Ink, f.Value, true)),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return borderedAction(t, gtx, ck, "copy", t.P.Rule, t.P.Dim)
							}),
						)
					})
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	})
}

// rawCard: the bytes, as hex or as the printable ASCII alone.
func (p *packetPanel) rawCard(t *theme.Theme, pk *state.Packet) layout.Widget {
	return comp.Card(t, "", func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(comp.Text(t, t.Sz.Section, t.P.Ink,
						fmt.Sprintf("Raw - %d lines", len(pk.RawLines)))),
					layout.Flexed(1, comp.Spacer),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Right: t.Sp.XS}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								return p.hexBtn.Layout(t, gtx, "Hex", "", !p.ascii, t.P.Accent)
							})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.ascBtn.Layout(t, gtx, "ASCII", "", p.ascii, t.P.Accent)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				var kids []layout.FlexChild
				for _, l := range pk.RawLines {
					line := l
					if p.ascii {
						// The dump's tail: offset and the printable column.
						if len(line) > 54 {
							line = line[:6] + line[54:]
						}
					}
					kids = append(kids, layout.Rigid(
						comp.Mono(t, t.Sz.Caption, t.P.Dim, line)))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
			}),
		)
	})
}

// journey: the message across every relay - heard and missed, named.
func (p *packetPanel) journey(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
	type jRow struct {
		at    uint32
		hop   int
		names string
		heard bool
	}
	var rows []jRow
	for _, h := range pk.Journey {
		if len(h.Heard) > 0 {
			rows = append(rows, jRow{h.AtMs, h.Hops, nameList(h.Heard, 4), true})
		}
		if len(h.MissedBy) > 0 {
			rows = append(rows, jRow{h.AtMs, h.Hops, nameList(h.MissedBy, 4), false})
		}
		if len(h.Heard) == 0 && len(h.MissedBy) == 0 {
			rows = append(rows, jRow{h.AtMs, h.Hops, "nobody in range", false})
		}
	}
	head := func(w int, s string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			px := gtx.Dp(unitDp(w))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = px, px
			d := comp.Text(t, t.Sz.Caption, t.P.Faint, s)(gtx)
			d.Size.X = px
			return d
		})
	}
	g := buildHopGraph(pk.Origin, pk.Journey)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return comp.CellGrid(t, gtx, 220, []layout.Widget{
				statBox(t, "heard", fmt.Sprintf("%d", pk.Heard),
					fmt.Sprintf("%d distinct nodes over the whole journey", pk.Reached)),
				statBox(t, "missed", fmt.Sprintf("%d", pk.Missed),
					"decode failures across every relay"),
			})
		}),
		// The propagation, drawn. Above the table rather than instead of it:
		// the picture answers what shape, the rows answer what exactly, and
		// somebody chasing a packet wants both within one glance.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return p.graphBtn.Layout(t, gtx, "graph", "",
								!p.noGraph, t.P.Accent)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: t.Sp.S}.Layout(gtx,
								comp.Text(t, t.Sz.Caption, t.P.Faint, graphCaption(g)))
						}),
					)
				})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.noGraph || len(g.Nodes) == 0 {
				return layout.Dimensions{}
			}
			return drawHopGraph(t, gtx, g)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.S, Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{}.Layout(gtx,
						head(70, "Time"), head(50, "Hop"),
						layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, "Heard by / Missed by")),
						layout.Flexed(1, comp.Spacer),
						layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, "Result")),
					)
				})
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return comp.List(t, &p.jList, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
				r := rows[i]
				word, col := "missed", t.P.Bad
				if r.heard {
					word, col = "heard", t.P.Good
				}
				return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								px := gtx.Dp(70)
								gtx.Constraints.Min.X, gtx.Constraints.Max.X = px, px
								d := comp.Mono(t, t.Sz.Caption, t.P.Dim,
									fmt.Sprintf("%.2f s", float64(r.at)/1000))(gtx)
								d.Size.X = px
								return d
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								px := gtx.Dp(50)
								gtx.Constraints.Min.X, gtx.Constraints.Max.X = px, px
								d := comp.Mono(t, t.Sz.Caption, t.P.Dim,
									fmt.Sprintf("%d", r.hop))(gtx)
								d.Size.X = px
								return d
							}),
							layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Ink, r.names, false)),
							layout.Rigid(comp.Text(t, t.Sz.Caption, col, word)),
						)
					})
			})(gtx)
		}),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, fmt.Sprintf(
			"%d transmissions, sorted by time - one message, followed by its "+
				"payload; the bytes differ at every hop", pk.Transmissions))),
	)
}

// nameList joins the first few names and counts the rest.
func nameList(names []string, keep int) string {
	if len(names) <= keep {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:keep], ", ") + fmt.Sprintf(", +%d more", len(names)-keep)
}

// ledger: the radio-level truth per receiver, yes and no in their colours,
// with the way into the link that explains each row.
func (p *packetPanel) ledger(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
	cols := []struct {
		label string
		width int
	}{
		{"Node", 180}, {"Offered", 70}, {"RSSI", 76}, {"SNR", 70},
		{"Demod", 64}, {"CRC", 56}, {"Firmware", 110}, {"", 60},
	}
	cell := func(w int, wgt layout.Widget) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			px := gtx.Dp(unitDp(w))
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
				ck, ok := p.whyBtns[r.Node]
				if !ok {
					ck = &widget.Clickable{}
					p.whyBtns[r.Node] = ck
				}
				if ck.Clicked(gtx) && p.do != nil {
					// Why?: select the pair, so Link and Budget answer about
					// exactly this path.
					p.do("nodes.select_many", []string{r.From, r.Node})
					p.do("budget.for_selection", nil)
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
								return borderedAction(t, gtx, ck, "why?", t.P.Rule, t.P.Dim)
							}),
						)
					})
			})(gtx)
		}),
	)
}

// whereItWent: every node's outcome for this one transmission.
func (p *packetPanel) whereItWent(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
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

// footer: where you are among the packets, and the two actions.
func (p *packetPanel) footer(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
	return layout.Inset{Top: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
				fmt.Sprintf("packet %d of many", pk.ID))),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: t.Sp.S, Right: t.Sp.XS}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions { return p.prev.Layout(t, gtx) })
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.next.Layout(t, gtx)
			}),
			layout.Flexed(1, comp.Spacer),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: t.Sp.S}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions { return p.copyBtn.Layout(t, gtx) })
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.close.Layout(t, gtx)
			}),
		)
	})
}

// packetText is the whole view as text, for the copy button.
func packetText(pk *state.Packet) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Packet #%d from %s at %.2f s - heard by %d, missed by %d\n",
		pk.ID, pk.Origin, float64(pk.AtMs)/1000, pk.Heard, pk.Missed)
	fmt.Fprintf(&b, "route %s  payload %s  version %s", pk.RouteType, pk.PayloadType, pk.Version)
	if pk.Transport != "" {
		fmt.Fprintf(&b, "  transport %s", pk.Transport)
	}
	b.WriteString("\n")
	if len(pk.Path) > 0 {
		fmt.Fprintf(&b, "path: %s\n", strings.Join(pk.Path, " -> "))
	}
	for _, f := range pk.PayloadFields {
		fmt.Fprintf(&b, "%s: %s\n", f.Name, f.Value)
	}
	for _, l := range pk.RawLines {
		b.WriteString(l)
		b.WriteString("\n")
	}
	return b.String()
}
