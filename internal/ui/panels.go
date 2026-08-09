package ui

import (
	"fmt"
	"math"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/coverage"
	"github.com/A13xB0/meshcoresim/internal/pathview"
	"github.com/A13xB0/meshcoresim/internal/planning"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

func (a *App) drawInspector() {
	imgui.SeparatorText("Nodes")
	imgui.TextWrapped("click on the map to select; ctrl-click a second for a link")

	// The list is a companion to the map, not the primary way in, so it
	// scrolls and does not try to show four hundred nodes at once.
	if imgui.BeginChildStrV("##nodelist", imgui.NewVec2(0, 160), 0, 0) {
		a.drawNodeRows()
	}
	imgui.EndChild()

	a.drawSelected()
}

func (a *App) drawNodeRows() {
	for i := range a.Nodes {
		n := &a.Nodes[i]
		label := fmt.Sprintf("%s##%d", n.Name, i)

		// An observer is drawn differently because it behaves differently: it
		// transmits nothing, so it can never be one end of a link budget.
		if n.Kind == scenario.SDRObserver {
			imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.55, 0.75, 0.95, 1))
		}
		selected := i == a.selected || i == a.linkTo
		if imgui.SelectableBoolPtr(label, &selected) {
			a.SelectNode(i, imgui.CurrentIO().KeyCtrl())
		}
		if n.Kind == scenario.SDRObserver {
			imgui.PopStyleColor()
		}

		imgui.SameLine()
		imgui.TextDisabled(kindLabel(n.Kind))
	}
}

func (a *App) drawSelected() {
	imgui.Spacing()
	imgui.SeparatorText("Selected")
	if a.selected < 0 {
		imgui.TextDisabled("nothing selected")
		return
	}
	n := &a.Nodes[a.selected]
	imgui.Text(n.Name)
	imgui.Text(fmt.Sprintf("%.5f, %.5f", n.Position.Lat, n.Position.Lon))

	changed := sliderF64("height m", &n.HeightAGLm, 1, 60, "%.1f")
	if n.Kind.Transmits() {
		changed = sliderF64("tx dBm", &n.TxPowerDBm, 2, 30, "%.0f") || changed
	} else {
		imgui.TextDisabled("an SDR observer transmits nothing")
	}
	if changed {
		// Recompute rather than cache. The height slider is the whole point of
		// the panel — "what would five more metres buy" — and an answer that
		// lags the slider by a frame is an answer to the previous question.
		a.recompute()
	}
	if ground, ok := a.Terrain.ElevationM(n.Position.Lat, n.Position.Lon); ok {
		imgui.TextDisabled(fmt.Sprintf("ground %.0f m, antenna top %.0f m AMSL",
			ground, ground+n.HeightAGLm))
	} else {
		imgui.TextDisabled("no terrain here - download tiles for this area")
	}
}

// SelectNode picks a node, or adds a second one to make a link.
//
// The modifier is a parameter rather than read from imgui here, so the
// selection rules can be tested without a window — which matters, because they
// are the only stateful thing in the UI and the drawing around them cannot be
// tested at all.
func (a *App) SelectNode(i int, addToLink bool) {
	if i < 0 || i >= len(a.Nodes) {
		return
	}
	if addToLink && a.selected >= 0 && a.selected != i {
		a.linkTo = i
	} else {
		a.selected, a.linkTo = i, -1
	}
	a.recompute()
}

// Link reports the current selection: the two node indices, or -1 where there
// is no selection.
func (a *App) Link() (from, to int) { return a.selected, a.linkTo }

// CutThrough is the current path analysis, or nil when no link is selected.
func (a *App) CutThrough() *pathview.CutThrough { return a.cut }

// Error is the reason the last analysis failed, empty if it did not.
func (a *App) Error() string { return a.cutErr }

// recompute rebuilds everything derived from the selection.
//
// Everything, every time. Caching a profile and invalidating it on the fields
// that "matter" is how a stale answer survives a change nobody thought was
// relevant — and the whole computation is a few milliseconds.
func (a *App) recompute() {
	a.cut, a.cutErr = nil, ""
	if a.selected < 0 || a.linkTo < 0 {
		return
	}
	from, to := a.Nodes[a.selected], a.Nodes[a.linkTo]
	c, err := pathview.Analyse(a.Terrain,
		from.Position.Lat, from.Position.Lon, from.HeightAGLm,
		to.Position.Lat, to.Position.Lon, to.HeightAGLm,
		a.freqMHz, 300)
	if err != nil {
		a.cutErr = err.Error()
		return
	}
	a.cut = &c
}

func (a *App) drawAnalysis() {
	if a.selected < 0 || a.linkTo < 0 {
		imgui.SeparatorText("Link")
		imgui.TextDisabled("select a node, then ctrl-click a second one")
		return
	}
	from, to := a.Nodes[a.selected], a.Nodes[a.linkTo]

	// An observer cannot be an end of a link budget, and saying so is better
	// than computing a number for a node that transmits nothing.
	if !from.Kind.Transmits() || !to.Kind.Transmits() {
		imgui.SeparatorText("Link")
		imgui.TextWrapped("An SDR observer transmits nothing, so there is no link budget to " +
			"compute. Observers are for looking at the spectrum, not for being one end of a path.")
		return
	}

	imgui.SeparatorText(fmt.Sprintf("%s  <->  %s", from.Name, to.Name))
	if a.cutErr != "" {
		imgui.TextWrapped(a.cutErr)
		return
	}
	if a.cut == nil {
		return
	}

	a.drawBudget(from, to)
	imgui.Spacing()
	a.drawCutThrough(*a.cut)
	imgui.Spacing()
	imgui.TextWrapped(a.cut.Verdict())
}

func (a *App) drawBudget(from, to scenario.Node) {
	r := &coverage.Raster{
		South: to.Position.Lat, North: to.Position.Lat,
		West: to.Position.Lon, East: to.Position.Lon,
		Width: 1, Height: 1, FreqMHz: a.freqMHz,
	}
	fixed := coverage.Endpoint{
		Name: from.Name, Lat: from.Position.Lat, Lon: from.Position.Lon,
		HeightAGLm: from.HeightAGLm, TxPowerDBm: from.TxPowerDBm, SensitivityDBm: -137,
		// The node's real antenna, evaluated in the real direction. A constant
		// here would silently make a Yagi omnidirectional, and the panel would
		// keep producing confident numbers.
		GainTowardsDBi: from.Antenna.GainTowardsDBi,
	}
	opts := coverage.Options{
		RemoteHeightAGLm: to.HeightAGLm, RemoteTxPowerDBm: to.TxPowerDBm,
		RemoteGainDBi:        to.Antenna.Pattern.PeakDBi() - to.Antenna.FeedlineDB,
		RemoteSensitivityDBm: -137, ProfileStepM: 30,
	}
	if err := coverage.Compute(fixed, a.Terrain, r, opts); err != nil {
		imgui.TextWrapped(err.Error())
		return
	}
	cell := r.At(0, 0)
	if cell.NoData {
		imgui.TextWrapped("No terrain covers this path.")
		return
	}
	l := planning.Summarise(from.Name, to.Name, a.cut.DistanceKm, cell)

	// Both directions, always, and side by side so neither can be read alone.
	if imgui.BeginTableV("##budget", 3, imgui.TableFlagsBordersInnerV, imgui.NewVec2(0, 0), 0) {
		imgui.TableNextRow()
		imgui.TableSetColumnIndex(0)
		imgui.Text(fmt.Sprintf("%.1f km", a.cut.DistanceKm))
		imgui.TableSetColumnIndex(1)
		marginText(fmt.Sprintf("%s -> %s", from.Name, to.Name), cell.OutboundMarginDB)
		imgui.TableSetColumnIndex(2)
		marginText(fmt.Sprintf("%s -> %s", to.Name, from.Name), cell.InboundMarginDB)
		imgui.EndTable()
	}
	if l.OneWayOnly {
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.65, 0.2, 1))
		imgui.TextWrapped("ONE WAY ONLY - one end will hear the other and not be heard back. " +
			"This is usually the most useful thing to know about a path.")
		imgui.PopStyleColor()
	}
}

func marginText(label string, dB float64) {
	imgui.TextDisabled(label)
	col := imgui.NewVec4(0.4, 0.85, 0.45, 1)
	if dB < 0 {
		col = imgui.NewVec4(0.9, 0.4, 0.4, 1)
	} else if dB < 6 {
		// Marginal is its own colour. Rounding a borderline link up to "works"
		// is the failure this whole project is arranged against.
		col = imgui.NewVec4(0.95, 0.72, 0.25, 1)
	}
	imgui.PushStyleColorVec4(imgui.ColText, col)
	imgui.Text(fmt.Sprintf("%+.1f dB", dB))
	imgui.PopStyleColor()
}

// drawCutThrough is the side-on view: terrain, sight line and Fresnel zone.
//
// The Fresnel zone is drawn because leaving it out is what makes a grazing path
// look clear. A user who can see the sight line passing a hilltop with nothing
// around it has been told the wrong thing.
func (a *App) drawCutThrough(c pathview.CutThrough) {
	avail := imgui.ContentRegionAvail()
	w := math.Max(320, float64(avail.X))
	// Take the height that is going spare, leaving room for the verdict below.
	// The profile is the main thing on this screen and a fixed 220 px left most
	// of the window empty.
	h := math.Max(220, float64(avail.Y)-110)

	pos := imgui.CursorScreenPos()
	dl := imgui.WindowDrawList()
	x0, y0 := float64(pos.X), float64(pos.Y)

	dl.AddRectFilled(imgui.NewVec2(float32(x0), float32(y0)),
		imgui.NewVec2(float32(x0+w), float32(y0+h)), colour(0.09, 0.10, 0.13, 1))

	lo, hi := c.Extent()
	span := hi - lo
	if span <= 0 {
		return
	}
	toY := func(m float64) float32 { return float32(y0 + h - (m-lo)/span*h) }
	toX := func(d float64) float32 {
		return float32(x0 + d/(c.DistanceKm*1000)*w)
	}

	// Fresnel zone first, so terrain and the sight line draw over it.
	//
	// One closed path rather than a quad per sample: adjacent translucent quads
	// leave a seam at every shared edge, and 300 of them read as vertical
	// stripes rather than a band.
	dl.PathClear()
	for i := range c.Samples {
		s := c.Samples[i]
		dl.PathLineTo(imgui.NewVec2(toX(s.DistM), toY(s.LOSm+s.FresnelM)))
	}
	for i := len(c.Samples) - 1; i >= 0; i-- {
		s := c.Samples[i]
		dl.PathLineTo(imgui.NewVec2(toX(s.DistM), toY(s.LOSm-s.FresnelM)))
	}
	// Concave rather than convex: the band is a lens that follows the sight
	// line, and a convex fill on a shape that is not convex renders as a
	// plausible but wrong wedge.
	dl.PathFillConcave(colour(0.25, 0.45, 0.75, 0.20))

	// Terrain with earth curvature, filled to the bottom. Opaque, so quads are
	// fine here — the seams that show through a translucent fill do not.
	for i := 1; i < len(c.Samples); i++ {
		p, q := c.Samples[i-1], c.Samples[i]
		dl.AddQuadFilled(
			imgui.NewVec2(toX(p.DistM), toY(p.BulgedM)),
			imgui.NewVec2(toX(q.DistM), toY(q.BulgedM)),
			imgui.NewVec2(toX(q.DistM), float32(y0+h)),
			imgui.NewVec2(toX(p.DistM), float32(y0+h)),
			colour(0.30, 0.34, 0.28, 1))
	}

	// The two nodes, drawn as masts standing on their own ground, with the sight
	// line running between their antenna tops.
	//
	// Without these the line appears to start in mid-air at the edge of the
	// plot, and there is nothing to show that it begins at an antenna rather
	// than at the terrain. The whole picture is about where the antennas are
	// relative to what is between them, so not drawing them leaves out one of
	// the three things.
	first, last := c.Samples[0], c.Samples[len(c.Samples)-1]
	drawMast := func(s pathview.Sample, alt float64) {
		x := toX(s.DistM)
		// Nudge the end masts inward so they are not clipped by the plot edge.
		if x <= float32(x0)+3 {
			x = float32(x0) + 3
		}
		if x >= float32(x0+w)-3 {
			x = float32(x0+w) - 3
		}
		dl.AddLineArgs(
			imgui.NewVec2(x, toY(s.GroundM)),
			imgui.NewVec2(x, toY(alt)),
			colour(0.85, 0.87, 0.92, 0.9), 2)
		dl.AddCircleFilled(imgui.NewVec2(x, toY(alt)), 4, colour(0.95, 0.96, 1, 1))
	}

	dl.AddLineArgs(
		imgui.NewVec2(toX(first.DistM), toY(c.TxAltM)),
		imgui.NewVec2(toX(last.DistM), toY(c.RxAltM)),
		colour(0.95, 0.95, 0.95, 0.9), 1.5)
	drawMast(first, c.TxAltM)
	drawMast(last, c.RxAltM)

	// The deciding point, marked. A picture with no marked point leaves the
	// reader to guess which hill mattered, and the answer is frequently not the
	// tallest one.
	if c.Worst >= 0 {
		wS := c.Samples[c.Worst]
		x := toX(wS.DistM)
		col := colour(0.95, 0.72, 0.25, 1)
		if c.Blocked {
			col = colour(0.9, 0.35, 0.35, 1)
		}
		dl.AddLineArgs(imgui.NewVec2(x, float32(y0)), imgui.NewVec2(x, float32(y0+h)), col, 1)
		dl.AddCircleFilled(imgui.NewVec2(x, toY(wS.BulgedM)), 4, col)
		// The obstruction is marked on the terrain it is part of, and the ends
		// are marked at their antennas. Three different things, three different
		// marks, so the picture cannot be misread as one of them.
	}

	imgui.Dummy(imgui.NewVec2(float32(w), float32(h)))
	imgui.TextDisabled(fmt.Sprintf(
		"terrain includes earth curvature; blue band is the first Fresnel zone   |   "+
			"%.1f km, %.0f-%.0f m", c.DistanceKm, lo, hi))
}

func colour(r, g, b, alpha float32) uint32 {
	return imgui.ColorConvertFloat4ToU32(imgui.NewVec4(r, g, b, alpha))
}

func kindLabel(k scenario.Kind) string {
	switch k {
	case scenario.SDRObserver:
		return "observer"
	case scenario.Companion:
		return "companion"
	case scenario.AdvancedRepeater:
		return "advanced"
	default:
		return "repeater"
	}
}

// sliderF64 bridges the model's float64 to imgui's float32.
//
// Written out rather than done with a helper returning *float32: that returns a
// pointer to a copy, so the slider moves, the value changes, and the model never
// hears about it. The bug is invisible — the UI looks right and simply does
// nothing.
func sliderF64(label string, v *float64, min, max float32, format string) bool {
	f := float32(*v)
	if imgui.SliderFloatV(label, &f, min, max, format, 0) {
		*v = float64(f)
		return true
	}
	return false
}
