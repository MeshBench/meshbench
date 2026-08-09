package ui

import (
	"context"
	"fmt"
	"math"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/antenna"
	"github.com/A13xB0/meshcoresim/internal/scenario"
	"github.com/A13xB0/meshcoresim/internal/terrain"
)

// Tool is what a click on the map does.
type Tool int

const (
	// ToolSelect picks nodes and drags the view. The default, because looking
	// is what people do most and a workbench that starts in a placement mode
	// scatters nodes across the map before anyone has read anything.
	ToolSelect Tool = iota
	ToolPlaceRepeater
	ToolPlaceCompanion
	ToolPlaceObserver
	ToolPlaceCustom
)

func (t Tool) label() string {
	switch t {
	case ToolPlaceRepeater:
		return "repeater"
	case ToolPlaceCompanion:
		return "companion"
	case ToolPlaceObserver:
		return "SDR observer"
	case ToolPlaceCustom:
		return "custom emitter"
	default:
		return "select"
	}
}

// drawMap is the central panel: terrain, nodes, and everything you can do to
// them.
func (a *App) drawMap(w, h float32) {
	if w < 32 || h < 32 {
		return
	}
	a.view.Width, a.view.Height = int(w), int(h)
	if a.view.MetresPerPixel <= 0 {
		a.view.FitTo(a.Nodes, int(w), int(h))
	}

	origin := imgui.CursorScreenPos()
	imgui.InvisibleButtonV("##map", imgui.NewVec2(w, h), 0)
	hovered := imgui.IsItemHovered()

	a.handleMapInput(origin, hovered)
	a.drawTerrain(origin, w, h)
	a.drawNodes(origin)
	a.drawScale(origin, h)
}

func (a *App) handleMapInput(origin imgui.Vec2, hovered bool) {
	io := imgui.CurrentIO()
	mouse := imgui.MousePos()
	mx := float64(mouse.X - origin.X)
	my := float64(mouse.Y - origin.Y)

	if hovered {
		if wheel := io.MouseWheel(); wheel != 0 {
			a.view.ZoomAt(mx, my, math.Pow(1.25, float64(wheel)))
			a.terrainDirty = true
		}
	}

	// Dragging always pans, whatever the tool. A map you cannot move while
	// holding a placement tool is a map you have to keep switching modes to use.
	if imgui.IsMouseDraggingV(imgui.MouseButtonLeft, 3) ||
		imgui.IsMouseDraggingV(imgui.MouseButtonMiddle, 3) {
		d := imgui.MouseDragDeltaV(imgui.MouseButtonLeft, 3)
		if d.X == 0 && d.Y == 0 {
			d = imgui.MouseDragDeltaV(imgui.MouseButtonMiddle, 3)
		}
		if d.X != 0 || d.Y != 0 {
			a.view.PanPixels(float64(d.X), float64(d.Y))
			a.terrainDirty = true
			a.dragged = true
			imgui.ResetMouseDragDeltaV(imgui.MouseButtonLeft)
			imgui.ResetMouseDragDeltaV(imgui.MouseButtonMiddle)
		}
	}

	if !hovered || !imgui.IsMouseReleased(imgui.MouseButtonLeft) {
		return
	}
	// A click that ended a drag is not a click. Without this every pan places a
	// node or changes the selection at wherever the drag stopped.
	if a.dragged {
		a.dragged = false
		return
	}

	if a.tool == ToolSelect {
		if i := a.view.NodeAt(a.Nodes, mx, my, 12); i >= 0 {
			a.SelectNode(i, io.KeyCtrl())
		}
		return
	}
	lat, lon := a.view.ScreenToLatLon(mx, my)
	a.placeNode(a.tool, lat, lon)
}

// placeNode adds a node where the user clicked.
func (a *App) placeNode(t Tool, lat, lon float64) {
	board, err := scenario.BoardByName(a.placeBoard)
	if err != nil {
		a.status = err.Error()
		return
	}
	n := scenario.Node{
		Position:      scenario.LatLon{Lat: lat, Lon: lon},
		Radio:         scenario.RadioConfig{CentreHz: a.freqMHz * 1e6, BandwidthHz: 250e3, SpreadFactor: 10, CodingRate: 1},
		NoiseFigureDB: board.NoiseFigureDB,
		Antenna: antenna.Mounted{
			Pattern:      antenna.Collinear{GainDBiPeak: board.AntennaDBi + 4},
			Polarisation: "vertical", FeedlineDB: board.FeedlineDB,
		},
	}
	switch t {
	case ToolPlaceCompanion:
		n.Kind, n.HeightAGLm, n.TxPowerDBm = scenario.Companion, 2, 14
		n.Antenna.Pattern = antenna.Dipole{}
	case ToolPlaceObserver:
		// No transmit power. An observer that transmits is not an observer, and
		// Validate refuses it — better to be unable to create one at all.
		n.Kind, n.HeightAGLm = scenario.SDRObserver, 5
	case ToolPlaceCustom:
		// Not necessarily MeshCore. A mast carrying something else is still an
		// RF participant, and the point of a workbench is to model the site
		// rather than only the network.
		n.Kind, n.HeightAGLm, n.TxPowerDBm = scenario.SimpleRepeater, 20, 27
	default:
		n.Kind, n.HeightAGLm, n.TxPowerDBm = scenario.SimpleRepeater, 10, board.MaxTxDBm
	}
	n.Name = a.uniqueName(t.label())

	if err := n.Validate(); err != nil {
		a.status = err.Error()
		return
	}
	a.Nodes = append(a.Nodes, n)
	a.status = fmt.Sprintf("placed %s at %.5f, %.5f", n.Name, lat, lon)
	a.SelectNode(len(a.Nodes)-1, false)
	// A new node changes every path loss in the network, so the run starts
	// again rather than continuing with the geometry of a mesh that no longer
	// exists.
	a.buildEngine()
}

func (a *App) uniqueName(prefix string) string {
	for i := 1; ; i++ {
		name := fmt.Sprintf("%s-%d", prefix, i)
		taken := false
		for _, n := range a.Nodes {
			if n.Name == name {
				taken = true
				break
			}
		}
		if !taken {
			return name
		}
	}
}

// drawTerrain paints whatever is under the nodes.
//
// Imagery is drawn as tiles, straight to the GPU. The hillshade is still a
// single generated image because it is computed from the DEM rather than
// downloaded, and it is only produced when it is the thing being shown.
func (a *App) drawTerrain(origin imgui.Vec2, w, h float32) {
	dl := imgui.WindowDrawList()
	dl.AddRectFilled(origin, imgui.NewVec2(origin.X+w, origin.Y+h), colour(0.09, 0.10, 0.13, 1))

	if a.composite.HasBase {
		a.tiles.upload(a.backend)
		a.drawTiles(origin, w, h, a.composite.Base)
		if a.composite.HasLabels {
			a.drawTiles(origin, w, h, a.composite.Labels)
		}
		return
	}

	// Hillshade.
	if a.terrainDirty || a.terrainTex == nil || a.terrainW != int(w) || a.terrainH != int(h) {
		a.regenerateTerrain(int(w), int(h))
	}
	a.uploadPendingTerrain()
	if a.terrainTex != nil {
		dl.AddImage(*a.terrainTex, origin, imgui.NewVec2(origin.X+w, origin.Y+h))
	}
}

// regenerateTerrain renders the hillshade off the render thread.
//
// Even from cache a 500x300 shade is 150,000 samples and several milliseconds,
// which is a visible stutter on every pan. The texture is uploaded by whichever
// frame finds one ready, because creating a GPU texture from another goroutine
// is not safe.
func (a *App) regenerateTerrain(w, h int) {
	a.terrainDirty = false
	a.terrainW, a.terrainH = w, h
	if a.rendering {
		return
	}
	a.rendering = true
	view := a.view
	go func() {
		a.pending <- terrainImage(a.Terrain, view, 3)
	}()
}

// uploadPendingTerrain takes a finished hillshade, if there is one.
func (a *App) uploadPendingTerrain() {
	select {
	case img := <-a.pending:
		a.rendering = false
		if img == nil || a.backend == nil {
			return
		}
		tex := a.backend.CreateTextureRgba(img, img.Bounds().Dx(), img.Bounds().Dy())
		a.terrainTex = &tex
	default:
	}
}

func (a *App) drawNodes(origin imgui.Vec2) {
	dl := imgui.WindowDrawList()
	from, to := a.Link()

	for i, n := range a.Nodes {
		x, y := a.view.LatLonToScreen(n.Position.Lat, n.Position.Lon)
		p := imgui.NewVec2(origin.X+float32(x), origin.Y+float32(y))

		col := colour(0.45, 0.85, 0.5, 1) // repeater
		switch n.Kind {
		case scenario.SDRObserver:
			col = colour(0.45, 0.72, 0.95, 1)
		case scenario.Companion:
			col = colour(0.95, 0.80, 0.35, 1)
		}

		// Uncertain positions are drawn as the circle they actually are. A dot
		// for a node known to five kilometres claims a precision nobody has.
		if n.UncertaintyKm > 0.2 {
			r := float32(n.UncertaintyKm * 1000 / a.view.MetresPerPixel)
			dl.AddCircleFilled(p, r, colour(0.95, 0.72, 0.25, 0.12))
		}

		if i == from || i == to {
			dl.AddCircleFilled(p, 9, colour(1, 1, 1, 0.35))
		}
		dl.AddCircleFilled(p, 5, col)
		dl.AddTextVec2V(imgui.NewVec2(p.X+8, p.Y-7), colour(0.92, 0.93, 0.96, 0.95), n.Name)
	}

	// The selected link, drawn on the map so the profile below has something to
	// refer to.
	if from >= 0 && to >= 0 {
		fx, fy := a.view.LatLonToScreen(a.Nodes[from].Position.Lat, a.Nodes[from].Position.Lon)
		tx, ty := a.view.LatLonToScreen(a.Nodes[to].Position.Lat, a.Nodes[to].Position.Lon)
		col := colour(0.9, 0.4, 0.4, 0.9)
		if c := a.CutThrough(); c != nil && !c.Blocked {
			col = colour(0.5, 0.9, 0.55, 0.9)
		}
		dl.AddLineArgs(
			imgui.NewVec2(origin.X+float32(fx), origin.Y+float32(fy)),
			imgui.NewVec2(origin.X+float32(tx), origin.Y+float32(ty)), col, 2)
	}
}

// drawScale puts a bar on the map. A map without one gives no sense of whether
// a gap is two hundred metres or twenty kilometres, which is the first thing
// anyone needs to know.
func (a *App) drawScale(origin imgui.Vec2, h float32) {
	dl := imgui.WindowDrawList()

	// A round number of metres that lands near 120 px.
	target := 120 * a.view.MetresPerPixel
	step := math.Pow(10, math.Floor(math.Log10(target)))
	for _, mult := range []float64{1, 2, 5, 10} {
		if step*mult >= target {
			step *= mult
			break
		}
	}
	px := float32(step / a.view.MetresPerPixel)

	y := origin.Y + h - 22
	x := origin.X + 14
	dl.AddLineArgs(imgui.NewVec2(x, y), imgui.NewVec2(x+px, y), colour(0.95, 0.96, 1, 0.9), 2)
	dl.AddLineArgs(imgui.NewVec2(x, y-4), imgui.NewVec2(x, y+4), colour(0.95, 0.96, 1, 0.9), 2)
	dl.AddLineArgs(imgui.NewVec2(x+px, y-4), imgui.NewVec2(x+px, y+4), colour(0.95, 0.96, 1, 0.9), 2)

	label := fmt.Sprintf("%.0f m", step)
	if step >= 1000 {
		label = fmt.Sprintf("%.0f km", step/1000)
	}
	dl.AddTextVec2V(imgui.NewVec2(x, y-20), colour(0.95, 0.96, 1, 0.9), label)

	// Attribution, on the map, for as long as the data is on it.
	dl.AddTextVec2V(imgui.NewVec2(x, y+8), colour(0.85, 0.87, 0.92, 0.75), a.attribution())
}

// fetchVisibleTerrain downloads the tiles for what is on screen.
//
// In the background, with progress, and never as a side effect of panning.
func (a *App) fetchVisibleTerrain() {
	fetcher, ok := a.Terrain.(interface {
		Estimate(south, north, west, east float64) terrain.Estimate
		Prefetch(ctx context.Context, south, north, west, east float64) error
	})
	if !ok {
		a.status = "this terrain source cannot download"
		return
	}
	if a.fetching {
		return
	}
	south, north, west, east := a.view.Bounds()
	est := fetcher.Estimate(south, north, west, east)
	if est.ToFetch == 0 {
		a.status = "already have this area"
		return
	}

	a.fetching = true
	a.fetchStatus = fmt.Sprintf("0/%d tiles", est.ToFetch)
	go func() {
		err := fetcher.Prefetch(context.Background(), south, north, west, east)
		a.fetchMu.Lock()
		a.fetching = false
		if err != nil {
			a.fetchStatus = err.Error()
		} else {
			a.fetchStatus = ""
			a.status = fmt.Sprintf("downloaded %d tiles", est.ToFetch)
		}
		a.terrainDirty = true
		a.fetchMu.Unlock()
	}()
}

// fetchState is what the toolbar shows while a download runs.
func (a *App) fetchState() string {
	a.fetchMu.Lock()
	defer a.fetchMu.Unlock()
	if !a.fetching {
		return ""
	}
	return a.fetchStatus
}
