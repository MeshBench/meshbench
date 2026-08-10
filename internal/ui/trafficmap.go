package ui

import (
	"sort"

	"github.com/AllenDang/cimgui-go/imgui"
)

// trafficTrailMs is how long a hop stays on the map after it happens. Long
// enough to see a flood's shape, short enough that the map is about now.
const trafficTrailMs = 2500

// drawTrafficLines paints recent hops on the map, HopReach-style: a line per
// reception, colour by what happened, fading as it ages.
//
// The timeline says what happened; this says *where*. A flood crossing a
// country is a shape — waves moving outward, a valley nothing crosses, one
// repeater bridging two clusters — and no table communicates a shape.
func (a *App) drawTrafficLines(origin imgui.Vec2, w, h float32) {
	if a.eng == nil {
		return
	}
	events := a.events()
	if len(events) == 0 {
		return
	}
	now := a.scrubMs
	var oldest uint32
	if now > trafficTrailMs {
		oldest = now - trafficTrailMs
	}

	// Events are appended in time order, so the window is found by search
	// rather than by scanning a hundred-thousand-row ledger every frame.
	lo := sort.Search(len(events), func(i int) bool { return events[i].AtMs >= oldest })

	pos := map[string]imgui.Vec2{}
	for i := range a.Nodes {
		x, y := a.view.LatLonToScreen(a.Nodes[i].Position.Lat, a.Nodes[i].Position.Lon)
		pos[a.Nodes[i].Name] = imgui.NewVec2(origin.X+float32(x), origin.Y+float32(y))
	}

	dl := imgui.WindowDrawList()
	dl.PushClipRectV(origin, imgui.NewVec2(origin.X+w, origin.Y+h), true)
	defer dl.PopClipRect()

	for _, ev := range events[lo:] {
		if ev.AtMs > now {
			break
		}
		// The same ticks as the timeline: one idea of "what am I looking at",
		// not a map that disagrees with the table under it.
		if !a.evShow[classify(ev)] {
			continue
		}
		// Age fades: a hop at full brightness just happened, a ghost is about
		// to leave. The animation is nothing more than this.
		alpha := 1 - float32(now-ev.AtMs)/float32(trafficTrailMs)

		switch classify(ev) {
		case evTx:
			// An expanding ring at the transmitter: the wavefront leaving.
			p, ok := pos[ev.From]
			if !ok {
				continue
			}
			r := 8 + (1-alpha)*26
			dl.AddCircleV(p, r, colour(0.7, 0.75, 0.9, 0.5*alpha), 0, 1.5)

		case evRx:
			a.hopLine(dl, pos, ev.From, ev.To, colour(0.45, 0.85, 0.5, 0.85*alpha), 2)

		case evInterference:
			a.hopLine(dl, pos, ev.From, ev.To, colour(0.95, 0.72, 0.25, 0.7*alpha), 1.5)

		case evHalfDuplex:
			a.hopLine(dl, pos, ev.From, ev.To, colour(0.6, 0.65, 0.95, 0.6*alpha), 1.5)
		}
		// Below-floor misses draw nothing: a line to somewhere the signal never
		// reached would draw exactly the clutter the ledger stopped recording.
	}
}

// drawTrafficKey is the legend: what each line means, with its filter tick.
//
// The ticks are the timeline's own filters, so hiding interference here hides
// it there too — the key is not a second filter to keep in sync, it is the
// same one drawn where the lines are.
func (a *App) drawTrafficKey(origin imgui.Vec2, w, h float32) {
	if a.eng == nil || len(a.events()) == 0 {
		return
	}
	a.ensureEvFilters()

	kw, kh := float32(190), float32(118)
	imgui.SetCursorScreenPos(imgui.NewVec2(origin.X+w-kw-10, origin.Y+h-kh-10))
	if imgui.BeginChildStrV("##traffickey", imgui.NewVec2(kw, kh), 0, imgui.WindowFlagsNoScrollbar) {
		textDim("traffic")
		dl := imgui.WindowDrawList()
		rows := []struct {
			class evClass
			label string
			col   uint32
			ring  bool
		}{
			{evTx, "transmission", colour(0.7, 0.75, 0.9, 0.9), true},
			{evRx, "received", colour(0.45, 0.85, 0.5, 0.95), false},
			{evInterference, "lost to interference", colour(0.95, 0.72, 0.25, 0.9), false},
			{evHalfDuplex, "half duplex", colour(0.6, 0.65, 0.95, 0.9), false},
		}
		for _, r := range rows {
			pos := imgui.CursorScreenPos()
			// The swatch is the thing itself: a ring for transmissions, a line
			// with a direction head for hops.
			mid := imgui.NewVec2(pos.X+11, pos.Y+9)
			if r.ring {
				dl.AddCircleV(mid, 6, r.col, 0, 1.5)
			} else {
				a1 := imgui.NewVec2(pos.X+2, pos.Y+9)
				a2 := imgui.NewVec2(pos.X+20, pos.Y+9)
				dl.AddLineArgs(a1, a2, r.col, 2)
				dl.AddLineArgs(imgui.NewVec2(pos.X+16, pos.Y+9), a2, r.col, 5)
			}
			imgui.SetCursorScreenPos(imgui.NewVec2(pos.X+26, pos.Y))
			imgui.Checkbox(r.label+"##key", &a.evShow[r.class])
		}
	}
	imgui.EndChild()
}

func (a *App) hopLine(dl *imgui.DrawList, pos map[string]imgui.Vec2, from, to string, col uint32, thick float32) {
	p1, ok1 := pos[from]
	p2, ok2 := pos[to]
	if !ok1 || !ok2 {
		return
	}
	dl.AddLineArgs(p1, p2, col, thick)
	// A short head at the receiving end says which way the hop went without
	// the cost of a real arrowhead per line.
	mid := imgui.NewVec2(p1.X+(p2.X-p1.X)*0.9, p1.Y+(p2.Y-p1.Y)*0.9)
	dl.AddLineArgs(mid, p2, col, thick*2.5)
}
