// The events panel, redesigned: cards, chips, causes, and a detail pane.
//
// The old table said what happened; this says what it means. The cards give
// the run's shape at a glance - how much of the traffic was lost, and to what
// - the chips filter by exactly those causes, and clicking a row explains it
// and offers the packet view.
package main

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// eventClasses is the chip order, which is also the card order.
var eventClasses = []string{"sent", "received", "half-duplex", "interference", "floor"}

type eventsPanel struct {
	allChip comp.Chip
	chips   [5]comp.Chip
	filter  string
	search  comp.Field
	list    widget.List
	rows    map[string]*widget.Clickable
	// follow keeps the newest row on screen; pausing stops the chase so a
	// row can be read while the run floods more in.
	follow   bool
	pauseBtn comp.Button
	openBtn  comp.Button
	closeBtn comp.Button
	sel      *state.Event
	selKey   string
	built    bool
	compact  bool
	forNode  bool
	// OnOpenPacket opens the packet view for a row's transmission.
	OnOpenPacket func(id uint64)
}

func (p *eventsPanel) build() {
	p.follow = true
	p.search.Hint = "search events"
	p.search.Editor.SingleLine = true
	p.pauseBtn.Label, p.pauseBtn.Kind = "pause", comp.Secondary
	p.openBtn.Label, p.openBtn.Kind = "open packet view", comp.Primary
	p.closeBtn.Label, p.closeBtn.Kind = "close", comp.Quiet
	p.rows = map[string]*widget.Clickable{}
	p.list.Axis = layout.Vertical
	p.built = true
}

// classCount reads one class's whole-run count.
func classCount(c state.EventCounts, class string) int {
	switch class {
	case "sent":
		return c.Sent
	case "received":
		return c.Received
	case "half-duplex":
		return c.HalfDuplex
	case "interference":
		return c.Interference
	case "floor":
		return c.Floor
	}
	return 0
}

// pct is a share of the whole run, in the cards' words.
func pct(n, total int) string {
	if total == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.2f%%", 100*float64(n)/float64(total))
}

func (p *eventsPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.built {
		p.build()
	}
	if s == nil {
		return layout.Dimensions{}
	}
	// The chips.
	if p.allChip.Click.Clicked(gtx) {
		p.filter = ""
	}
	for i := range p.chips {
		if p.chips[i].Click.Clicked(gtx) {
			p.filter = eventClasses[i]
		}
	}
	if p.pauseBtn.Click.Clicked(gtx) {
		p.follow = !p.follow
	}
	p.pauseBtn.Label = "pause"
	if !p.follow {
		p.pauseBtn.Label = "follow"
	}

	// The visible rows: the tail, filtered by chip and search. The selected
	// node scopes the compact variant, which is what the Inspector shows.
	sel := selectedNodeName(s)
	want := strings.ToLower(fieldText(&p.search))
	shown := make([]*state.Event, 0, len(s.Events))
	for i := range s.Events {
		e := &s.Events[i]
		if p.filter != "" && e.Class != p.filter {
			continue
		}
		if p.forNode && sel != "" && e.From != sel && e.To != sel {
			continue
		}
		if want != "" && !strings.Contains(strings.ToLower(e.From), want) &&
			!strings.Contains(strings.ToLower(e.To), want) &&
			!strings.Contains(strings.ToLower(e.Detail), want) {
			continue
		}
		shown = append(shown, e)
	}

	var kids []layout.FlexChild
	if !p.compact {
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.cards(t, gtx, s)
		}))
	}
	kids = append(kids,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.chipRow(t, gtx, s)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.headerRow(t, gtx)
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.table(t, gtx, shown)
		}),
	)
	if p.sel != nil && !p.compact {
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.detail(t, gtx)
		}))
	}
	kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return p.footer(t, gtx, len(shown), s)
	}))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
}

// cards is the run's shape: totals and causes, with their share.
func (p *eventsPanel) cards(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	total := s.Counts.Total()
	cells := []layout.Widget{
		comp.StatCell(t, "Total events", fmt.Sprintf("%d", s.EventTotal), ""),
		comp.StatCell(t, "Duration", fmt.Sprintf("%.2f s", float64(s.NowMs)/1000), ""),
	}
	for _, class := range eventClasses {
		class := class
		n := classCount(s.Counts, class)
		cells = append(cells, comp.StatCell(t, comp.ClassLabel(class),
			fmt.Sprintf("%d", n), pct(n, total)))
	}
	return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, comp.Card(t, "",
		func(gtx layout.Context) layout.Dimensions {
			return comp.CellGrid(t, gtx, 110, cells)
		}))
}

// chipRow filters by cause, with the search beside it.
func (p *eventsPanel) chipRow(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	kids := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return p.allChip.Layout(t, gtx, "All events",
						fmt.Sprintf("%d", s.EventTotal), p.filter == "", t.P.Accent)
				})
		}),
	}
	for i := range p.chips {
		i := i
		class := eventClasses[i]
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			n := classCount(s.Counts, class)
			if p.compact && (class == "half-duplex" || class == "interference" || class == "floor") &&
				n == 0 {
				return layout.Dimensions{}
			}
			return layout.Inset{Right: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return p.chips[i].Layout(t, gtx, comp.ClassLabel(class),
						fmt.Sprintf("%d", n), p.filter == class, comp.ClassColour(t, class))
				})
		}))
	}
	kids = append(kids, layout.Flexed(1, comp.Spacer))
	if !p.compact {
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(200)
			return p.search.Layout(t, gtx)
		}))
	}
	return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
		})
}

// The columns, sized once. Time and SNR are numbers and sit in mono.
func (p *eventsPanel) colWidths(gtx layout.Context) (tw, fw, snr, pill int) {
	tw, fw, snr, pill = gtx.Dp(64), gtx.Dp(130), gtx.Dp(56), gtx.Dp(96)
	if p.compact {
		fw, pill = gtx.Dp(96), gtx.Dp(24)
	}
	return
}

func (p *eventsPanel) headerRow(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	tw, fw, snr, pill := p.colWidths(gtx)
	cell := func(w int, label string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
			d := comp.Text(t, t.Sz.Caption, t.P.Faint, label)(gtx)
			d.Size.X = w
			return d
		})
	}
	kids := []layout.FlexChild{
		cell(tw, "Time"), cell(fw, "From"), cell(fw, "To"), cell(snr, "SNR (dB)"),
	}
	if !p.compact {
		// The compact view's type column is a dot; a header wider than the
		// column wraps at the panel edge, so it goes unlabelled there.
		kids = append(kids,
			cell(pill, "Type"),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, "What happened")))
	}
	return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx, kids...)
		})
}

// eventKey identifies one event across frames well enough to keep a selection.
func eventKey(e *state.Event) string {
	return fmt.Sprintf("%d/%s/%s/%d/%s", e.PacketID, e.From, e.To, e.AtMs, e.Kind)
}

func (p *eventsPanel) table(t *theme.Theme, gtx layout.Context, shown []*state.Event) layout.Dimensions {
	p.list.ScrollToEnd = p.follow
	tw, fw, snr, pill := p.colWidths(gtx)
	return comp.List(t, &p.list, len(shown), func(gtx layout.Context, i int) layout.Dimensions {
		e := shown[i]
		key := eventKey(e)
		ck, ok := p.rows[key]
		if !ok {
			ck = &widget.Clickable{}
			p.rows[key] = ck
		}
		if ck.Clicked(gtx) {
			if p.selKey == key {
				p.sel, p.selKey = nil, ""
			} else {
				copyOf := *e
				p.sel, p.selKey = &copyOf, key
			}
		}
		return ck.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			macro := func(gtx layout.Context) layout.Dimensions {
				cell := func(w int, wgt layout.Widget) layout.FlexChild {
					return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
						d := wgt(gtx)
						d.Size.X = w
						return d
					})
				}
				kids := []layout.FlexChild{
					cell(tw, comp.Mono(t, t.Sz.Caption, t.P.Dim,
						fmt.Sprintf("%.2f s", float64(e.AtMs)/1000))),
					cell(fw, comp.OneLine(t, t.Sz.Caption, t.P.Ink, e.From, false)),
					cell(fw, comp.OneLine(t, t.Sz.Caption, t.P.Ink, e.To, false)),
					cell(snr, comp.Mono(t, t.Sz.Caption, t.P.Dim, snrOf(e))),
				}
				if p.compact {
					kids = append(kids, cell(pill, func(gtx layout.Context) layout.Dimensions {
						return classDot(t, gtx, e.Class)
					}))
				} else {
					kids = append(kids,
						cell(pill, comp.TypePill(t, e.Class)),
						layout.Flexed(1, comp.OneLine(t, t.Sz.Caption,
							theme.Alpha(comp.ClassColour(t, e.Class), 0.9), e.Detail, false)),
					)
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
			}
			if p.selKey == key {
				return selected(t, gtx, macro)
			}
			return layout.Inset{Top: t.Sp.XXS, Bottom: t.Sp.XXS}.Layout(gtx, macro)
		})
	})(gtx)
}

// detail is the pane beneath the table: the row, explained, with the way into
// the packet view.
func (p *eventsPanel) detail(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	e := p.sel
	if p.openBtn.Click.Clicked(gtx) && p.OnOpenPacket != nil && e.PacketID != 0 {
		p.OnOpenPacket(e.PacketID)
	}
	if p.closeBtn.Click.Clicked(gtx) {
		p.sel, p.selKey = nil, ""
		return layout.Dimensions{}
	}
	kv := func(k, v string) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(76)
					gtx.Constraints.Max.X = gtx.Dp(76)
					return comp.Text(t, t.Sz.Caption, t.P.Faint, k)(gtx)
				}),
				layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Ink, v)),
			)
		}
	}
	return layout.Inset{Top: t.Sp.S}.Layout(gtx, comp.Card(t, "",
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return comp.CellGrid(t, gtx, 150, []layout.Widget{
						kv("Time", fmt.Sprintf("%.2f s", float64(e.AtMs)/1000)),
						kv("From", e.From),
						kv("To", e.To),
						kv("SNR", snrOf(e)),
						kv("Message", fmt.Sprintf("0x%X", e.MessageID)),
						kv("Packet", fmt.Sprintf("#%d", e.PacketID)),
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: t.Sp.M}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(comp.Text(t, t.Sz.Caption,
									comp.ClassColour(t, e.Class), "What happened")),
								layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Ink, e.Detail)),
								layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
									explainClass(e.Class))),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: t.Sp.S}.Layout(gtx,
										func(gtx layout.Context) layout.Dimensions {
											return layout.Flex{}.Layout(gtx,
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													return p.openBtn.Layout(t, gtx)
												}),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													return layout.Inset{Left: t.Sp.S}.Layout(gtx,
														func(gtx layout.Context) layout.Dimensions {
															return p.closeBtn.Layout(t, gtx)
														})
												}),
											)
										})
								}),
							)
						})
				}),
			)
		}))
}

func (p *eventsPanel) footer(t *theme.Theme, gtx layout.Context, shown int, s *state.Snapshot) layout.Dimensions {
	label := fmt.Sprintf("showing %d of %d events", shown, s.EventTotal)
	if shown < len(s.Events) {
		label += " (filtered)"
	} else if s.EventTotal > len(s.Events) {
		label += " - the rest are older than the tail the interface keeps"
	}
	return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, label)),
				layout.Flexed(1, comp.Spacer),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.pauseBtn.Layout(t, gtx)
				}),
			)
		})
}

// explainClass is the second sentence under a detail: what this class of
// outcome means for the mesh, not just for this packet.
func explainClass(class string) string {
	switch class {
	case "sent":
		return "A transmission occupies the channel for its own airtime; everything else transmitting in that window collides."
	case "received":
		return "Received and decoded; whether it changed anything is the firmware's decision."
	case "half-duplex":
		return "A radio cannot hear while transmitting. This is a cost of relaying, not a fault."
	case "interference":
		return "The signal was strong enough on its own; a stronger transmission in the same window took the channel."
	case "floor":
		return "Too quiet to demodulate at this spreading factor. More height, more power, or a closer relay are the fixes."
	}
	return ""
}

// classDot is the compact table's type column: the colour without the word.
func classDot(t *theme.Theme, gtx layout.Context, class string) layout.Dimensions {
	return comp.Dot(comp.ClassColour(t, class), gtx.Dp(4))(gtx)
}

// selected wraps a row in the selection tint, drawn under the content.
func selected(t *theme.Theme, gtx layout.Context, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Top: t.Sp.XXS, Bottom: t.Sp.XXS}.Layout(gtx, w)
	call := macro.Stop()
	comp.FillRect(gtx, dims.Size, t.P.Selected)
	call.Add(gtx.Ops)
	return dims
}
