package ui

import (
	"fmt"
	"math"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/linkbudget"
)

// mapLayers is which optional overlays are drawn.
type mapLayers struct {
	patterns bool
	links    bool
	region   bool
	init     bool
}

// drawLayerControls is the map's own overlay switches.
//
// On the map rather than in the toolbar: they change what the map shows, and a
// control that lives away from the thing it controls is a control people stop
// associating with it. The toolbar was also simply full.
func (a *App) drawLayerControls(origin imgui.Vec2, w float32) {
	if !a.layers.init {
		a.layers.init, a.layers.links, a.layers.region = true, true, true
	}
	// At small map widths the strip collapses behind one button: three
	// checkboxes over a 400 px map cover the part being looked at.
	if w < 560 {
		imgui.SetCursorScreenPos(imgui.NewVec2(origin.X+w-70, origin.Y+8))
		if imgui.BeginChildStrV("##layersbtn", imgui.NewVec2(60, 26), 0, imgui.WindowFlagsNoScrollbar) {
			if imgui.SmallButton("layers") {
				imgui.OpenPopupStr("##layerspop")
			}
			if imgui.BeginPopup("##layerspop") {
				imgui.Checkbox("links", &a.layers.links)
				imgui.Checkbox("patterns", &a.layers.patterns)
				imgui.Checkbox("region", &a.layers.region)
				imgui.EndPopup()
			}
		}
		imgui.EndChild()
	} else {
		// Everything that decides what the map *shows*, on the map: the layer
		// toggles, the basemap picker, labels, fit and the terrain fetch. The
		// old toolbar row had them a screen away from the thing they change.
		imgui.SetCursorScreenPos(imgui.NewVec2(origin.X+w-560, origin.Y+8))
		if imgui.BeginChildStrV("##layers", imgui.NewVec2(550, imgui.FrameHeight()+10),
			imgui.ChildFlagsFrameStyle, imgui.WindowFlagsNoScrollbar) {
			imgui.Checkbox("links", &a.layers.links)
			imgui.SameLine()
			imgui.Checkbox("patterns", &a.layers.patterns)
			imgui.SameLine()
			imgui.Checkbox("region", &a.layers.region)
			imgui.SameLineV(0, 14)
			a.drawLayerPicker()
			imgui.SameLine()
			if imgui.SmallButton("fit") {
				a.view.FitTo(a.Nodes, a.view.Width, a.view.Height)
				a.terrainDirty = true
			}
			imgui.SameLine()
			// Downloading is an explicit act: a workbench that fetches whenever
			// the view moves is unusable on a tethered connection.
			if imgui.SmallButton("terrain") {
				a.fetchVisibleTerrain()
			}
			if imgui.IsItemHovered() {
				tip := "download elevation tiles for this view"
				if est, ok := a.terrainEstimate(); ok {
					tip += fmt.Sprintf("\n%d tiles, %d cached, roughly %d MB",
						est.Tiles, est.Cached, est.BytesRough/1_000_000)
				}
				imgui.SetTooltip(tip)
			}
			if s := a.fetchState(); s != "" {
				imgui.SameLine()
				textDim(s)
			}
		}
		imgui.EndChild()
	}

	// The node filter lives on the map, like every map tool: matches are
	// highlighted in place, because a list of names answers "is it here" while
	// the map answers "where".
	// Clear of the tool rail, which owns the left edge.
	left := a.railRight + 10
	if left < origin.X+8 {
		left = origin.X + 8
	}
	imgui.SetCursorScreenPos(imgui.NewVec2(left, origin.Y+8))
	if imgui.BeginChildStrV("##mapfilter", imgui.NewVec2(210, imgui.FrameHeight()+10),
		imgui.ChildFlagsFrameStyle, imgui.WindowFlagsNoScrollbar) {
		imgui.SetNextItemWidth(-1)
		imgui.InputTextWithHint("##mapfilterbox", "filter nodes", &a.nodeFilter, 0, nil)
	}
	imgui.EndChild()
}

// drawAntennaPatterns draws each node's radiation pattern, rotated to its real
// bearing.
//
// "So a null pointing at your neighbour is visible rather than deduced." A
// directional antenna's gain is a shape, and a scalar in an inspector cannot
// show that the shape is aimed at the wrong hill.
func (a *App) drawAntennaPatterns(origin imgui.Vec2, w, h float32) {
	if !a.layers.patterns {
		return
	}
	dl := imgui.WindowDrawList()
	dl.PushClipRectV(origin, imgui.NewVec2(origin.X+w, origin.Y+h), true)
	defer dl.PopClipRect()

	for i := range a.Nodes {
		n := &a.Nodes[i]
		if n.Antenna.Pattern == nil || !n.Kind.Transmits() {
			continue
		}
		x, y := a.view.LatLonToScreen(n.Position.Lat, n.Position.Lon)
		if x < -100 || y < -100 || x > float64(w)+100 || y > float64(h)+100 {
			continue
		}
		centre := imgui.NewVec2(origin.X+float32(x), origin.Y+float32(y))

		// Scaled so the peak is a fixed size on screen: the shape is the
		// information, not the absolute gain, and a pattern that shrank as you
		// zoomed out would be unreadable exactly when the map is most useful.
		const peakPx = 34.0
		peak := n.Antenna.Pattern.PeakDBi()
		var prev imgui.Vec2
		for step := 0; step <= 72; step++ {
			az := float64(step) * 5
			// Relative to the mount's bearing, which is what rotates the shape.
			g := n.Antenna.Pattern.GainDBi(normalise(az-n.Antenna.BearingDeg), 0)
			// 20 dB of dynamic range, floored: a null is a small radius, not a
			// negative one.
			r := (g - (peak - 20)) / 20 * peakPx
			if r < 2 {
				r = 2
			}
			rad := (az - 90) * math.Pi / 180
			p := imgui.NewVec2(
				centre.X+float32(math.Cos(rad)*r),
				centre.Y+float32(math.Sin(rad)*r))
			if step > 0 {
				dl.AddLineArgs(prev, p, colour(0.55, 0.75, 0.95, 0.5), 1)
			}
			prev = p
		}
	}
}

func normalise(deg float64) float64 {
	d := math.Mod(deg, 360)
	if d > 180 {
		d -= 360
	}
	if d < -180 {
		d += 360
	}
	return d
}

// drawLinkLines draws every workable pair, coloured *and dashed* by outcome.
//
// Colour is never the only channel — engineers include colourblind engineers,
// and this UI leans heavily on status colour. Solid is decoded, dashed is
// marginal, dotted is no path.
func (a *App) drawLinkLines(origin imgui.Vec2, w, h float32) {
	if !a.layers.links || a.neighboursOf == "" {
		return
	}
	sel := a.nodeIndex(a.neighboursOf)
	if sel < 0 {
		a.neighboursOf = ""
		return
	}
	dl := imgui.WindowDrawList()
	dl.PushClipRectV(origin, imgui.NewVec2(origin.X+w, origin.Y+h), true)
	defer dl.PopClipRect()

	src := a.Nodes[sel]
	sx, sy := a.view.LatLonToScreen(src.Position.Lat, src.Position.Lon)
	from := imgui.NewVec2(origin.X+float32(sx), origin.Y+float32(sy))
	shown := 0

	// One node's neighbours, asked for deliberately. Every pair in a 600-node
	// network is 180,000 lines and a terrain profile each, and the question
	// people ask is "what can *this* node reach".
	for i := range a.Nodes {
		if i == sel || !a.Nodes[i].Kind.Transmits() {
			continue
		}
		dst := a.Nodes[i]
		dx, dy := a.view.LatLonToScreen(dst.Position.Lat, dst.Position.Lon)
		if dx < 0 && sx < 0 || dy < 0 && sy < 0 {
			continue
		}
		to := imgui.NewVec2(origin.X+float32(dx), origin.Y+float32(dy))

		margin, ok := a.linkMargin(sel, i)
		if !ok {
			continue
		}
		// Neighbours, not "every other node with a computed path loss". A line
		// to somewhere the signal cannot reach is not information about a
		// neighbour — it is the same clutter the ledger stopped recording, and
		// on a 600-node map it was almost all of the lines.
		switch {
		case margin >= 6:
			dl.AddLineArgs(from, to, colour(0.45, 0.85, 0.5, 0.6), 1.6)
			shown++
		case margin >= 0:
			// Marginal is worth drawing and worth distinguishing: it is the
			// band where this model should not be trusted about the sign.
			dashed(dl, from, to, colour(0.95, 0.72, 0.25, 0.65), 1.6, 9, 5)
			shown++
		}
	}

	// The count as a number too. A picture of six lines is six lines; "6
	// neighbours" is the answer somebody was actually after.
	label := fmt.Sprintf("%d neighbours", shown)
	size := imgui.CalcTextSize(label)
	at := imgui.NewVec2(from.X-size.X/2, from.Y-26)
	dl.AddRectFilledV(imgui.NewVec2(at.X-4, at.Y-2),
		imgui.NewVec2(at.X+size.X+4, at.Y+size.Y+2), colour(0.05, 0.06, 0.08, 0.85), 3, 0)
	dl.AddTextVec2V(at, colour(0.85, 0.9, 1, 1), label)
}

// dashed draws a line in segments, so outcome is carried by pattern as well as
// by colour.
func dashed(dl *imgui.DrawList, a, b imgui.Vec2, col uint32, thick, on, off float32) {
	dx, dy := b.X-a.X, b.Y-a.Y
	length := float32(math.Hypot(float64(dx), float64(dy)))
	if length < 1 {
		return
	}
	ux, uy := dx/length, dy/length
	for t := float32(0); t < length; t += on + off {
		e := t + on
		if e > length {
			e = length
		}
		dl.AddLineArgs(
			imgui.NewVec2(a.X+ux*t, a.Y+uy*t),
			imgui.NewVec2(a.X+ux*e, a.Y+uy*e), col, thick)
	}
}

// linkMargin is the weaker direction's margin between two nodes, from the
// engine's own path loss so the map and the budget cannot disagree.
func (a *App) linkMargin(i, j int) (float64, bool) {
	if a.eng == nil {
		return 0, false
	}
	loss, ok := a.eng.PathLossForTest(i, j)
	if !ok {
		return 0, false
	}
	// Through internal/linkbudget, which the new interface uses too. Two
	// copies of a link budget drift in the direction of whichever one somebody
	// last looked at, and the map and the budget panel disagreeing about
	// whether a link closes is the worst version of that.
	return linkbudget.MarginDB(a.Nodes[i], a.Nodes[j], loss), true
}
