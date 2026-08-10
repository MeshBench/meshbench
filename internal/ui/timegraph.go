package ui

import (
	"fmt"
	"sort"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/engine"
)

// timeGraphState is the packet timeline's view of time.
type timeGraphState struct {
	// spanMs is the visible window. Zoomable from the whole run down to a
	// single symbol, because both are questions people ask.
	spanMs   float32
	centreMs float32
	follow   bool
	lanes    []string
}

// laneHeight is one node's row. Tall enough to click, short enough that twenty
// nodes fit before scrolling.
const laneHeight = 18

// drawTimeGraph is the packet timeline: one lane per node, transmissions as
// bars drawn to airtime scale.
//
// This is the panel that shows the mesh's actual behaviour rather than a list
// of things that happened to it — the flood spreading outward, the retransmit
// backoffs, and the moment two repeaters key up together. A table can tell you
// two transmissions overlapped; only this can show you by how much, and that
// the second one started while the first was still going out.
func (a *App) drawTimeGraph() {
	if a.eng == nil {
		imgui.TextDisabled("no simulation yet - press run in the strip above")
		return
	}
	events := a.events()
	if len(events) == 0 {
		imgui.TextDisabled("no traffic yet - press run, or schedule some")
		return
	}
	g := &a.tg
	if g.spanMs == 0 {
		g.spanMs, g.follow = 20_000, true
	}

	now := float32(a.eng.NowMs())
	if g.follow {
		g.centreMs = now - g.spanMs/2
	}

	// Controls first: what window am I looking at, and does it track the run.
	imgui.SetNextItemWidth(150)
	imgui.SliderFloatV("##span", &g.spanMs, 200, 120_000, "%.0f ms window",
		imgui.SliderFlagsLogarithmic)
	imgui.SameLine()
	imgui.Checkbox("follow", &g.follow)
	imgui.SameLine()
	if !g.follow {
		imgui.SetNextItemWidth(-90)
		imgui.SliderFloat("##centre", &g.centreMs, 0, now)
	} else {
		imgui.TextDisabled(fmt.Sprintf("t = %.2f s", now/1000))
	}

	// Lanes are the nodes that have done anything, in first-appearance order,
	// so a 600-node scenario shows the dozen that matter rather than 600 empty
	// rows.
	g.lanes = g.lanes[:0]
	seen := map[string]bool{}
	for _, ev := range events {
		for _, name := range [2]string{ev.From, ev.To} {
			if name != "" && !seen[name] {
				seen[name] = true
				g.lanes = append(g.lanes, name)
			}
		}
	}
	sort.Strings(g.lanes)
	laneOf := make(map[string]int, len(g.lanes))
	for i, n := range g.lanes {
		laneOf[n] = i
	}

	if !imgui.BeginChildStrV("##tg", imgui.NewVec2(0, 0), 0, imgui.WindowFlagsHorizontalScrollbar) {
		imgui.EndChild()
		return
	}
	origin := imgui.CursorScreenPos()
	avail := imgui.ContentRegionAvail()
	const gutter = 120 // room for node names
	plotW := avail.X - gutter
	if plotW < 80 {
		imgui.EndChild()
		return
	}
	dl := imgui.WindowDrawList()

	from := g.centreMs
	to := g.centreMs + g.spanMs
	x := func(ms float32) float32 { return origin.X + gutter + (ms-from)/(to-from)*plotW }

	// Lane backgrounds and names.
	for i, name := range g.lanes {
		y := origin.Y + float32(i*laneHeight)
		if i%2 == 1 {
			dl.AddRectFilledV(imgui.NewVec2(origin.X, y),
				imgui.NewVec2(origin.X+avail.X, y+laneHeight),
				colour(1, 1, 1, 0.025), 0, 0)
		}
		dl.AddTextVec2V(imgui.NewVec2(origin.X+2, y+2), colour(0.7, 0.74, 0.82, 1), name)
	}
	height := float32(len(g.lanes) * laneHeight)

	// A second-grid, so the axis is readable without a ruler.
	step := gridStepMs(g.spanMs)
	for t := float32(int(from/step)) * step; t <= to; t += step {
		if t < from {
			continue
		}
		dl.AddLineArgs(imgui.NewVec2(x(t), origin.Y), imgui.NewVec2(x(t), origin.Y+height),
			colour(1, 1, 1, 0.06), 1)
		dl.AddTextVec2V(imgui.NewVec2(x(t)+2, origin.Y+height), colour(0.5, 0.53, 0.6, 1),
			fmt.Sprintf("%.2fs", t/1000))
	}

	// Transmissions as bars to airtime scale; receptions as ticks on the
	// receiving lane, joined to the transmission that caused them.
	txAt := map[uint64]struct{ x, y float32 }{}
	for _, ev := range events {
		lane, ok := laneOf[laneName(ev)]
		if !ok {
			continue
		}
		y := origin.Y + float32(lane*laneHeight)

		if ev.Kind == "tx" {
			airtime := airtimeOf(ev)
			x0, x1 := x(float32(ev.AtMs)), x(float32(ev.AtMs)+airtime)
			if x1 < origin.X+gutter || x0 > origin.X+avail.X {
				continue
			}
			if x1-x0 < 2 {
				x1 = x0 + 2 // a short packet is still a packet
			}
			sel := a.pkt.selected && a.pkt.id == ev.PacketID
			col := colour(0.45, 0.62, 0.95, 0.85)
			if sel {
				col = colour(1, 1, 1, 0.95)
			}
			dl.AddRectFilledV(imgui.NewVec2(x0, y+3), imgui.NewVec2(x1, y+laneHeight-3), col, 2, 0)
			txAt[ev.PacketID] = struct{ x, y float32 }{x0, y + laneHeight/2}
			continue
		}

		px := x(float32(ev.AtMs))
		if px < origin.X+gutter || px > origin.X+avail.X {
			continue
		}
		if !a.evShow[classify(ev)] {
			continue
		}
		c := eventColour(ev)
		col := colour(c.X, c.Y, c.Z, 0.95)
		dl.AddRectFilledV(imgui.NewVec2(px-1, y+5), imgui.NewVec2(px+1, y+laneHeight-5), col, 0, 0)
		// The line back to the transmission is what makes a flood legible:
		// every reception is visibly caused by a particular bar.
		if src, ok := txAt[ev.PacketID]; ok && ev.Kind == "rx" {
			dl.AddLineArgs(imgui.NewVec2(src.x, src.y), imgui.NewVec2(px, y+laneHeight/2),
				colour(c.X, c.Y, c.Z, 0.22), 1)
		}
	}

	// The playhead.
	if now >= from && now <= to {
		dl.AddLineArgs(imgui.NewVec2(x(now), origin.Y), imgui.NewVec2(x(now), origin.Y+height),
			colour(0.95, 0.4, 0.4, 0.8), 1.5)
	}

	// Clicking a bar selects that packet, cross-highlighting it everywhere.
	imgui.InvisibleButtonV("##tghit", imgui.NewVec2(avail.X, height+16), 0)
	if imgui.IsItemHovered() && imgui.IsMouseClickedBool(imgui.MouseButtonLeft) {
		mouse := imgui.MousePos()
		ms := from + (mouse.X-origin.X-gutter)/plotW*(to-from)
		lane := int((mouse.Y - origin.Y) / laneHeight)
		if id, ok := packetAt(events, laneOf, lane, uint32(ms)); ok {
			a.pkt.id, a.pkt.selected = id, true
		}
	}
	imgui.EndChild()
}

// laneName is the node a row belongs to: the sender for a transmission, the
// receiver for anything else.
func laneName(ev engine.Event) string {
	if ev.Kind == "tx" {
		return ev.From
	}
	return ev.To
}

// airtimeOf recovers a transmission's airtime from the event's own text.
//
// The engine writes it there, and reading it back beats recomputing from
// assumptions this panel would have to make about the node's radio.
func airtimeOf(ev engine.Event) float32 {
	var bytes int
	var ms float32
	if _, err := fmt.Sscanf(ev.Detail, "%d bytes, %f ms on air", &bytes, &ms); err != nil {
		return 20
	}
	return ms
}

// packetAt finds the transmission under a click.
func packetAt(events []engine.Event, laneOf map[string]int, lane int, atMs uint32) (uint64, bool) {
	var best uint64
	var found bool
	for _, ev := range events {
		if ev.Kind != "tx" || laneOf[ev.From] != lane {
			continue
		}
		end := ev.AtMs + uint32(airtimeOf(ev))
		if atMs >= ev.AtMs && atMs <= end {
			best, found = ev.PacketID, true
		}
	}
	return best, found
}

// gridStepMs picks a round grid interval for the current zoom.
func gridStepMs(span float32) float32 {
	for _, s := range []float32{10, 50, 100, 500, 1000, 5000, 10_000, 30_000} {
		if span/s <= 12 {
			return s
		}
	}
	return 60_000
}
