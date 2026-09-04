// The events panel, redesigned: cards, chips, causes, and a detail pane.
//
// The old table said what happened; this says what it means. The cards give
// the run's shape at a glance - how much of the traffic was lost, and to what
// - the chips filter by exactly those causes, and clicking a row explains it
// and offers the packet view.
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

// eventClasses is the chip order, which is also the card order. It mirrors
// engine.Classes: a class the engine counts and this list omits is a cause an
// operator cannot see or filter on.
var eventClasses = [8]string{
	"sent", "received", "half-duplex", "interference",
	"collision", "receiver-busy", "floor", "unclassified",
}

type eventsPanel struct {
	allChip comp.Chip
	chips   [len(eventClasses)]comp.Chip
	filter  string
	search  comp.Field
	list    widget.List
	rows    map[string]*widget.Clickable
	// follow keeps the newest row on screen; pausing stops the chase so a
	// row can be read while the run floods more in.
	follow   bool
	pauseBtn comp.Button
	built    bool
	compact  bool
	forNode  bool
	// scopedTo is the node the view is filtered to this frame, for the
	// empty state to name.
	scopedTo string
	// pointedAt is the row under the pointer, said in full at the foot of the
	// panel. Rebuilt as the rows draw, so a pointer that has left the table
	// does not leave a stale row named down there.
	pointedAt string
	// OnOpenPacket opens the packet view for a row's transmission.
	OnOpenPacket func(id uint64)
}

func (p *eventsPanel) build() {
	p.follow = true
	p.search.Hint = "search events"
	p.search.Editor.SingleLine = true
	p.pauseBtn.Label, p.pauseBtn.Kind = "pause", comp.Secondary
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
	case "collision":
		return c.Collision
	case "receiver-busy":
		return c.ReceiverBusy
	case "floor":
		return c.Floor
	case "unclassified":
		return c.Unclassified
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
	sel := comp.SelectedNodeName(s)
	p.scopedTo = ""
	if p.forNode {
		p.scopedTo = sel
	}
	want := strings.ToLower(comp.FieldText(&p.search))
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
	p.keepRows(shown)

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
	kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return p.footer(t, gtx, len(shown), s)
	}))
	was := p.pointedAt
	d := layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	// The footer is a rigid and the rows are the flexed child, so the footer
	// is measured before the rows it reads: without asking for another frame,
	// a pointer that comes to rest on a row leaves the row unnamed.
	if was != p.pointedAt {
		gtx.Execute(op.InvalidateCmd{})
	}
	return d
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
			// The compact column fits three chips; the causes are already on
			// the rows as coloured dots, and a chip squeezed past the edge
			// folds into a vertical smear.
			if p.compact && class != "sent" && class != "received" {
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
//
// The compact view's names take whatever the panel actually has, rather than
// a width chosen for the narrowest rail. Two node names are what a row is
// about, and at 96dp several of a fixture's names cut to the same characters:
// "sco-fif-aberdo..." twice over is a pair of rows that cannot be told apart.
// Floored so a narrow rail still shows something of each, and capped because
// past a point the names are complete and the rest is white space.
func (p *eventsPanel) colWidths(gtx layout.Context) (tw, fw, snr, pill int) {
	tw, fw, snr, pill = gtx.Dp(64), gtx.Dp(130), gtx.Dp(56), gtx.Dp(96)
	if p.compact {
		pill = gtx.Dp(24)
		fw = (gtx.Constraints.Max.X - tw - snr - pill) / 2
		if lo := gtx.Dp(96); fw < lo {
			fw = lo
		}
		if hi := gtx.Dp(260); fw > hi {
			fw = hi
		}
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

// rowSlack is how many rows the click map may hold beyond what is on screen
// before it is rebuilt. A filter typed one character at a time narrows and
// widens the shown set on consecutive frames, and rebuilding on every one of
// those would be work for nothing.
const rowSlack = 128

// keepRows drops the click state of events that have left the view.
//
// One clickable per distinct event, and the events themselves are a tail the
// store discards from: over the multi-hour run this tool exists for, the
// panel most likely to be left open was also the only thing holding on to
// every event that had scrolled past. Rebuilding against what is shown gives
// that back, and keeps the presses that are actually in flight.
func (p *eventsPanel) keepRows(shown []*state.Event) {
	if len(p.rows) <= len(shown)+rowSlack {
		return
	}
	kept := make(map[string]*widget.Clickable, len(shown))
	for _, e := range shown {
		key := eventKey(e)
		if ck, ok := p.rows[key]; ok {
			kept[key] = ck
		}
	}
	p.rows = kept
}

// eventKey identifies one event across frames well enough to keep a selection.
func eventKey(e *state.Event) string {
	return fmt.Sprintf("%d/%s/%s/%d/%s", e.PacketID, e.From, e.To, e.AtMs, e.Kind)
}

func (p *eventsPanel) table(t *theme.Theme, gtx layout.Context, shown []*state.Event) layout.Dimensions {
	// Cleared here rather than at the top of the frame: the footer that reads
	// it is measured first, and clearing it before that would leave it with
	// nothing to say every other frame. Before the empty case, so a table that
	// has just emptied does not leave a row named underneath it.
	p.pointedAt = ""
	// An empty scoped view says what it is waiting for. "0 of 744
	// (filtered)" with a blank pane reads as broken; it is a node that has
	// not spoken - Alex read it exactly that way before working it out.
	if len(shown) == 0 && p.forNode {
		msg := "select a node: this shows what it said and heard"
		if p.scopedTo != "" {
			msg = "nothing from or to " + p.scopedTo +
				" in the recent events - it has not spoken yet"
		}
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint, msg))
	}
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
			// Straight to the packet window: the intermediate detail pane was
			// not what anybody expected a click to do, and Alex said so.
			if p.OnOpenPacket != nil && e.PacketID != 0 {
				p.OnOpenPacket(e.PacketID)
			}
		}
		return ck.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			if ck.Hovered() {
				p.pointedAt = e.From + " -> " + e.To
			}
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
			return layout.Inset{Top: t.Sp.XXS, Bottom: t.Sp.XXS}.Layout(gtx, macro)
		})
	})(gtx)
}

func (p *eventsPanel) footer(t *theme.Theme, gtx layout.Context, shown int, s *state.Snapshot) layout.Dimensions {
	label, col := fmt.Sprintf("showing %d of %d events", shown, s.EventTotal), t.P.Faint
	if shown < len(s.Events) {
		label += " (filtered)"
	} else if s.EventTotal > len(s.Events) {
		label += " - the rest are older than the tail the interface keeps"
	}
	// The row under the pointer wins the line: a From and a To cut to their
	// column are two names this panel is otherwise unable to say, and the
	// count is still there the moment the pointer moves off.
	if p.pointedAt != "" {
		label, col = p.pointedAt, t.P.Ink
	}
	return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, col, label, false)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.pauseBtn.Layout(t, gtx)
				}),
			)
		})
}

// classDot is the compact table's type column: the colour without the word.
func classDot(t *theme.Theme, gtx layout.Context, class string) layout.Dimensions {
	return comp.Dot(comp.ClassColour(t, class), gtx.Dp(4))(gtx)
}
