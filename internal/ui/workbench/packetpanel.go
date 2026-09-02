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

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

var packetTabs = []string{"Overview", "Dissection", "Journey", "Reception ledger", "Where it went"}

// packetOpenOnTab is which tab a freshly built Packet panel shows, set by
// -packet-tab.
//
// The same reason the node window has one: a tab that can only be reached by
// clicking is a tab no screenshot can capture and no script can drive, and the
// propagation graph lives on the second of these.
var packetOpenOnTab int

type packetPanel struct {
	tabs   [5]widget.Clickable
	tab    int
	scroll widget.List
	// overviewList scrolls the Overview tab, separately from Dissection: two
	// tabs sharing one scroll position jump when you switch between them.
	overviewList widget.List
	// selField is the Dissection row whose bytes are picked out in the hex
	// below it, or -1 for none. An index into the same order the table is
	// built in, reset whenever the packet changes - a row number means
	// nothing once a different frame is being read.
	selField int
	// selSpan is the chosen structural region, or -1. A span and a field are
	// the same kind of answer - a range of bytes - so only one is ever set.
	selSpan int
	// selFor is the packet selField belongs to.
	selFor  uint64
	jList   widget.List
	lList   widget.List
	fates   comp.Table
	whyBtns map[string]*widget.Clickable
	// whyOpen is the node the "why?" modal is answering for; empty is closed.
	// A name rather than an index because the row it was clicked from can
	// move or vanish under it - the packet view stays live while its message
	// keeps propagating.
	whyOpen  string
	whyClose comp.Button
	whyList  widget.List
	// graphBtn folds the propagation picture away. Docked in a third of a
	// window there is not room for both it and the table, and which one you
	// want depends on whether you are asking "what shape" or "what exactly".
	graphBtn comp.Chip
	noGraph  bool
	// gview is where the operator has dragged and zoomed the picture, how deep
	// they asked it to go, and which reasons they are looking at.
	gview graphView
	// winH is the Packet window's own height this frame, in pixels - captured
	// once at the top of Draw so the graph can size itself off the window
	// rather than off whatever its rigid siblings left over.
	winH     int
	hopChips [len(hopLimits)]comp.Chip
	// modeChips picks which of the three layouts is drawing the graph -
	// columns, radial or free-form - in the order graphModes lists them.
	modeChips [len(graphModes)]comp.Chip
	keyChips  [len(missKinds)]comp.Chip
	resetBtn  comp.Button
	showKind  map[missKind]bool
	copyBtn   comp.Button
	prev      comp.Button
	next      comp.Button
	close     comp.Button
	built     bool
	do        Do
}

func (p *packetPanel) build() {
	p.fates.Cols = []comp.Column{
		{Title: "t", Width: 64, Right: true, Mono: true, Sortable: true},
		{Title: "node", Width: 170, Sortable: true},
		{Title: "SNR", Width: 64, Right: true, Mono: true},
		{Title: "outcome"},
	}
	p.resetBtn.Label, p.resetBtn.Kind = "fit", comp.Quiet
	p.showKind = map[missKind]bool{}
	for _, mk := range missKinds {
		p.showKind[mk.Kind] = true
	}
	p.copyBtn.Label, p.copyBtn.Kind = "copy packet", comp.Secondary
	p.prev.Label, p.prev.Kind = "prev", comp.Secondary
	p.next.Label, p.next.Kind = "next", comp.Secondary
	p.close.Label, p.close.Kind = "close", comp.Primary
	p.whyBtns = map[string]*widget.Clickable{}
	p.whyClose.Label, p.whyClose.Kind = "close", comp.Quiet
	p.whyList.Axis = layout.Vertical
	p.scroll.Axis, p.jList.Axis, p.lList.Axis = layout.Vertical, layout.Vertical, layout.Vertical
	p.overviewList.Axis = layout.Vertical
	p.selField, p.selSpan = -1, -1
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
	p.winH = gtx.Constraints.Max.Y
	if p.selFor != pk.ID {
		p.selFor, p.selField, p.selSpan = pk.ID, -1, -1
	}
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
	if p.resetBtn.Click.Clicked(gtx) {
		p.gview.reset()
	}
	for i, n := range hopLimits {
		if p.hopChips[i].Click.Clicked(gtx) {
			p.gview.maxHops = n
			p.gview.reset()
		}
	}
	for i, gm := range graphModes {
		if p.modeChips[i].Click.Clicked(gtx) {
			p.gview.mode = gm.Mode
			p.gview.reset()
		}
	}
	for i, mk := range missKinds {
		if p.keyChips[i].Click.Clicked(gtx) {
			p.showKind[mk.Kind] = !p.showKind[mk.Kind]
		}
	}
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return p.layoutBody(t, gtx, pk)
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return p.whyModal(t, gtx, pk)
		}),
	)
}

func (p *packetPanel) layoutBody(t *theme.Theme, gtx layout.Context, pk *state.Packet) layout.Dimensions {
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
						return p.dissection(t, gtx, pk)
					case 2:
						return p.journey(t, gtx, pk)
					case 3:
						return p.ledger(t, gtx, pk)
					case 4:
						return p.whereItWent(t, gtx, pk)
					}
					return p.overview(t, gtx, pk)
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
		if i == 4 {
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
