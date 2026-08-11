package ui

import (
	"context"
	"fmt"
	"math"
	"strings"

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
	// ToolMove drags nodes. Its own mode on purpose: moving a repeater changes
	// every result on screen, and doing it by accident while panning is a
	// change nobody noticed making.
	ToolMove
	ToolPlaceRepeater
	ToolPlaceCompanion
	ToolPlaceObserver
	ToolPlaceCustom
)

// tip is the sentence behind the glyph: a palette of symbols needs one, and
// the label alone ("custom emitter") does not say what clicking will do.
func (t Tool) tip() string {
	switch t {
	case ToolMove:
		return "move: drag a node, and every link recomputes"
	case ToolPlaceRepeater:
		return "place a repeater"
	case ToolPlaceCompanion:
		return "place a companion - a user's device"
	case ToolPlaceObserver:
		return "place an SDR observer - listens, transmits nothing"
	case ToolPlaceCustom:
		return "place an interference source - raises nearby noise floors"
	default:
		return "select: click a node, ctrl-click a second for a link"
	}
}

func (t Tool) label() string {
	switch t {
	case ToolMove:
		return "move"
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
	// Active means the drag *started* on the map itself. The old check asked
	// only "is a drag happening", which panned the world underneath every
	// window being moved and every slider being pulled.
	active := imgui.IsItemActive()

	a.handleMapInput(origin, hovered, active)
	a.drawTerrain(origin, w, h)
	a.drawCoverage(origin, w, h)
	a.drawBoundaryOutline(origin, w, h)
	a.drawImportPreviewBox(origin, w, h)
	a.drawLinkLines(origin, w, h)
	a.drawAntennaPatterns(origin, w, h)
	a.drawTrafficLines(origin, w, h)
	a.drawNodes(origin, w, h)
	a.drawTrafficKey(origin, w, h)
	a.drawToolRail(origin, h)
	a.drawLayerControls(origin, w)

	// Overlays are positioned children, and each one leaves the layout cursor
	// wherever it finished. Whatever is drawn after the map — the tabs — would
	// otherwise appear at the last overlay's position, which collapsed the map
	// entirely when a control strip was added at the top.
	imgui.SetCursorScreenPos(imgui.NewVec2(origin.X, origin.Y+h))
	a.drawScale(origin, h)
}

func (a *App) handleMapInput(origin imgui.Vec2, hovered, active bool) {
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

	// Dragging a node moves it, and everything recomputes.
	//
	// The primary "what if": workflow A is drag a candidate onto the hill and
	// watch every link recolour. It must feel immediate, which is why the link
	// matrix warms in the background rather than being filled on demand.
	if a.tool == ToolMove && active && a.dragNode < 0 &&
		imgui.IsMouseDraggingV(imgui.MouseButtonLeft, 3) {
		if i := a.view.NodeAt(a.Nodes, startX(mx), startY(my), 14); i >= 0 {
			a.dragNode = i
		} else {
			a.dragNode = -2 // this drag is a pan, decided once at its start
		}
	}
	if a.dragNode >= 0 {
		if imgui.IsMouseDraggingV(imgui.MouseButtonLeft, 1) {
			lat, lon := a.view.ScreenToLatLon(mx, my)
			a.Nodes[a.dragNode].Position = scenario.LatLon{Lat: lat, Lon: lon}
			// The cache is keyed on node index, and this node's every path has
			// just changed. Dropping the whole matrix is cheaper than being
			// clever about which entries moved.
			a.onGeometryChanged()
		} else {
			a.dragNode = -1
			a.startWarm()
		}
		return
	}

	if active && imgui.IsMouseReleased(imgui.MouseButtonLeft) {
		a.dragNode = -1
	}

	// Panning: only a drag that began on the map and not on a node. A map you
	// cannot move while holding a placement tool is a map you keep switching
	// modes to use; a map that moves while you drag a node window over it is
	// worse.
	if active && (imgui.IsMouseDraggingV(imgui.MouseButtonLeft, 3) ||
		imgui.IsMouseDraggingV(imgui.MouseButtonMiddle, 3)) {
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

	if hovered && imgui.IsMouseReleased(imgui.MouseButtonRight) {
		// Context menus are the point of being a desktop application: the web
		// version of this is a mode you have to enter first.
		a.ctxNode = a.view.NodeAt(a.Nodes, mx, my, 12)
		a.ctxLat, a.ctxLon = a.view.ScreenToLatLon(mx, my)
		imgui.OpenPopupStr("##mapctx")
	}
	a.drawMapContext()

	// Delete removes the selection. The keyboard's job, not a button's: by the
	// time anyone wants a node gone they already have it selected.
	if hovered && imgui.IsKeyPressedBool(imgui.KeyDelete) {
		if from, _ := a.Link(); from >= 0 {
			a.DeleteNode(from)
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
		// Double-click opens the node's own window: the gesture every desktop
		// application uses for "show me this thing properly", and the one
		// people were already trying before finding the right-click menu.
		if imgui.IsMouseDoubleClicked(imgui.MouseButtonLeft) {
			if i := a.view.NodeAt(a.Nodes, mx, my, 12); i >= 0 {
				a.SelectNode(i, false)
				a.openNodeWindow(a.Nodes[i].Name)
				return
			}
		}
		if i := a.view.NodeAt(a.Nodes, mx, my, 12); i >= 0 {
			if io.KeyShift() {
				a.toggleMulti(i)
			} else {
				a.SelectNode(i, io.KeyCtrl())
			}
		}
		return
	}
	lat, lon := a.view.ScreenToLatLon(mx, my)
	a.placeNode(a.tool, lat, lon)
	// One click places one node. A tool that stays armed scatters accidental
	// repeaters across the map on every click meant as a selection; holding
	// shift keeps it armed on purpose, for laying out a chain.
	if !io.KeyShift() {
		a.tool = ToolSelect
	}
}

// drawMapContext is the right-click menu, over a node or over ground.
func (a *App) drawMapContext() {
	if !imgui.BeginPopup("##mapctx") {
		return
	}
	if a.ctxNode >= 0 && a.ctxNode < len(a.Nodes) {
		n := a.Nodes[a.ctxNode]
		textDim(n.Name)
		imgui.Separator()
		if imgui.MenuItemBool("open window") {
			a.openNodeWindow(n.Name)
			a.SelectNode(a.ctxNode, false)
		}
		if imgui.MenuItemBool("link from here") {
			a.SelectNode(a.ctxNode, false)
		}
		if from, _ := a.Link(); from >= 0 && from != a.ctxNode {
			if imgui.MenuItemBool("link to here") {
				a.SelectNode(a.ctxNode, true)
			}
		}
		if imgui.MenuItemBool("move this node") {
			a.tool = ToolMove
			a.SelectNode(a.ctxNode, false)
		}
		if imgui.MenuItemBool("show neighbours") {
			a.neighboursOf = n.Name
		}
		if a.neighboursOf != "" {
			if imgui.MenuItemBool("hide neighbours") {
				a.neighboursOf = ""
			}
		}
		if imgui.MenuItemBool("coverage from here") {
			a.startCoverage(a.ctxNode)
		}
		if imgui.MenuItemBool("provision this node") {
			a.provisionNode(a.ctxNode)
		}
		if imgui.IsItemHovered() {
			imgui.SetTooltip("Sends the Provisioning window's on-start commands to this\n" +
				"node's own CLI now. Needs its firmware running; opens the\n" +
				"Provisioning window otherwise.")
		}
		imgui.Separator()
		if imgui.MenuItemBool("delete " + n.Name) {
			a.DeleteNode(a.ctxNode)
		}
	} else {
		textDim(fmt.Sprintf("%.5f, %.5f", a.ctxLat, a.ctxLon))
		imgui.Separator()
		for _, t := range []Tool{ToolPlaceRepeater, ToolPlaceCompanion, ToolPlaceObserver, ToolPlaceCustom} {
			if imgui.MenuItemBool("place " + t.label() + " here") {
				a.placeNode(t, a.ctxLat, a.ctxLon)
			}
		}
		imgui.Separator()
		if a.cov.tex != nil {
			if imgui.MenuItemBool("clear coverage overlay") {
				a.clearCoverage()
			}
			imgui.SetNextItemWidth(120)
			imgui.SliderFloat("opacity", &a.cov.opacity, 0.1, 1)
		}
		if imgui.MenuItemBool("fit view to nodes") {
			a.view.FitTo(a.Nodes, a.view.Width, a.view.Height)
			a.terrainDirty = true
		}
	}
	imgui.EndPopup()
}

// DeleteNode removes a node from the scenario.
func (a *App) DeleteNode(i int) {
	if i < 0 || i >= len(a.Nodes) {
		return
	}
	name := a.Nodes[i].Name
	a.Nodes = append(a.Nodes[:i], a.Nodes[i+1:]...)
	// Indices above the hole all moved down one, and the selection may have
	// been the deleted node itself. Recomputing them is less error-prone than
	// adjusting: there are four cases and this is not the place to enumerate
	// them wrongly.
	a.selected, a.linkTo, a.msel = -1, -1, nil
	delete(a.consoles, name)
	delete(a.nodeWindows, name)
	a.status = "deleted " + name
	// Geometry changed, so the run starts over; a firmware process for the
	// deleted node would otherwise keep transmitting from a mast that is gone.
	a.buildEngine()
	a.selectFirstLink()
}

// placeNode adds a node where the user clicked.
func (a *App) placeNode(t Tool, lat, lon float64) {
	board, err := scenario.BoardByName(a.placeBoard)
	if err != nil {
		a.status = err.Error()
		return
	}
	n := scenario.Node{
		Board:         board.Name,
		Position:      scenario.LatLon{Lat: lat, Lon: lon},
		Radio:         a.defaultRadio(),
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
		// A real interference source, per ADR-0012 - not a repeater wearing a
		// costume. It contributes noise at every receiver through the same
		// terrain engine; it relays nothing and runs no firmware. On-channel
		// and narrowband by default, because "a PMR carrier on my frequency"
		// is the case people actually hit.
		n.Kind, n.HeightAGLm, n.TxPowerDBm = scenario.Emitter, 20, 44
		n.EmitterDutyPct = 100
		n.Radio.BandwidthHz = 25e3
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
	if a.terrainTex == nil {
		return
	}
	// Drawn where its ground is, not where the map rectangle is.
	//
	// The shade is generated off the frame thread and takes a moment; it was
	// then painted over the whole map whatever the view had done in the
	// meantime, so during a pan the hills sat still while the nodes moved,
	// and afterwards they were simply in the wrong place until a new one
	// arrived. Projecting its own corners through the current view makes it
	// slide and scale with everything else - stale in detail, never
	// misplaced.
	tl, br := a.terrainScreenRect(origin)
	dl.AddImage(*a.terrainTex, tl, br)
	// Ground the shade does not cover yet - after a zoom out, before the new
	// one lands - is left as background rather than stretched to fit, which
	// would be terrain drawn where it was never computed.
	if a.rendering && (tl.X > origin.X+2 || tl.Y > origin.Y+2 ||
		br.X < origin.X+w-2 || br.Y < origin.Y+h-2) {
		at := imgui.NewVec2(origin.X+10, origin.Y+h-24)
		dl.AddTextVec2(at, colour(0.55, 0.58, 0.65, 1), "shading the new view...")
	}
}

// terrainScreenRect is where the current hillshade's ground sits on screen.
func (a *App) terrainScreenRect(origin imgui.Vec2) (imgui.Vec2, imgui.Vec2) {
	v := a.terrainView
	if v.MetresPerPixel <= 0 || v.Width == 0 || v.Height == 0 {
		// Never recorded: it was rendered for this view, so it fills it.
		return origin, imgui.NewVec2(origin.X+float32(a.terrainW), origin.Y+float32(a.terrainH))
	}
	// The corners it was rendered for, in ground terms, projected through
	// the view being drawn now.
	northLat, westLon := v.ScreenToLatLon(0, 0)
	southLat, eastLon := v.ScreenToLatLon(float64(v.Width), float64(v.Height))
	x0, y0 := a.view.LatLonToScreen(northLat, westLon)
	x1, y1 := a.view.LatLonToScreen(southLat, eastLon)
	return imgui.NewVec2(origin.X+float32(x0), origin.Y+float32(y0)),
		imgui.NewVec2(origin.X+float32(x1), origin.Y+float32(y1))
}

// regenerateTerrain renders the hillshade off the render thread.
//
// Even from cache a 500x300 shade is 150,000 samples and several milliseconds,
// which is a visible stutter on every pan. The texture is uploaded by whichever
// frame finds one ready, because creating a GPU texture from another goroutine
// is not safe.
func (a *App) regenerateTerrain(w, h int) {
	a.terrainW, a.terrainH = w, h
	if a.rendering {
		// Still busy with the last one. The flag stays set so the next frame
		// tries again: clearing it here dropped every view change that
		// happened during a render, and the map then kept a shade for a view
		// nobody was looking at any more. That was invisible while the
		// texture was stretched over the whole map, and obvious the moment it
		// was drawn where its ground actually is.
		return
	}
	a.terrainDirty = false
	a.rendering = true
	view := a.view
	a.pendingView = view
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
		// The view this shade was rendered for, so it can be drawn where its
		// ground actually is rather than wherever the map happens to be now.
		a.terrainView = a.pendingView
	default:
	}
}

// labelRect is a placed label, kept so the next one can avoid it.
type labelRect struct{ x0, y0, x1, y1 float32 }

func (r labelRect) overlaps(o labelRect) bool {
	return r.x0 < o.x1 && o.x0 < r.x1 && r.y0 < o.y1 && o.y0 < r.y1
}

func (a *App) drawNodes(origin imgui.Vec2, w, h float32) {
	dl := imgui.WindowDrawList()
	from, to := a.Link()

	// Clipped to the map, exactly as the tiles are. The draw list has no idea
	// where the map ends, and without this a node just south of the visible
	// extent painted its marker and label over the tabs below — which read as
	// the UI falling apart rather than as a node being off screen.
	dl.PushClipRectV(origin, imgui.NewVec2(origin.X+w, origin.Y+h), true)
	defer dl.PopClipRect()

	// Labels are placed greedily and dropped where they would collide.
	//
	// Four hundred nodes on a county's worth of map put every name on top of
	// every other name, and the result is a solid block of text with a map
	// somewhere underneath it. Cartography solved this a long time ago: draw a
	// label only where it fits, and let zooming in reveal the rest.
	//
	// The selection is placed first and never dropped, because the one label
	// somebody is definitely looking at is the node they just clicked.
	placed := make([]labelRect, 0, 64)
	labelled := 0
	dropped := 0
	filtering := strings.TrimSpace(a.nodeFilter) != ""

	drawLabel := func(p imgui.Vec2, name string, force bool) {
		at := imgui.NewVec2(p.X+9, p.Y-7)
		size := imgui.CalcTextSize(name)
		box := labelRect{at.X - 3, at.Y - 1, at.X + size.X + 3, at.Y + size.Y + 1}
		if !force {
			for _, q := range placed {
				if box.overlaps(q) {
					dropped++
					return
				}
			}
		}
		placed = append(placed, box)
		labelled++
		// The label gets its own backing. White text was invisible over a light
		// basemap, which is exactly where the place names it has to compete with
		// already are.
		dl.AddRectFilledV(imgui.NewVec2(box.x0, box.y0), imgui.NewVec2(box.x1, box.y1),
			colour(0.05, 0.06, 0.08, 0.7), 3, 0)
		dl.AddTextVec2V(at, colour(0.95, 0.96, 1, 1), name)
	}

	// The selection first, so it wins every collision it is in.
	for _, i := range []int{from, to} {
		if i < 0 || i >= len(a.Nodes) {
			continue
		}
		n := a.Nodes[i]
		x, y := a.view.LatLonToScreen(n.Position.Lat, n.Position.Lon)
		drawLabel(imgui.NewVec2(origin.X+float32(x), origin.Y+float32(y)), n.Name, true)
	}

	for i, n := range a.Nodes {
		x, y := a.view.LatLonToScreen(n.Position.Lat, n.Position.Lon)
		// A margin, not the exact edge: a marker centred just off screen still
		// pokes its rim in, and a label can extend well right of its node.
		if x < -160 || y < -40 || x > float64(w)+40 || y > float64(h)+40 {
			continue
		}
		p := imgui.NewVec2(origin.X+float32(x), origin.Y+float32(y))

		col := colour(0.45, 0.85, 0.5, 1) // repeater
		switch n.Kind {
		case scenario.SDRObserver:
			col = colour(0.45, 0.72, 0.95, 1)
		case scenario.Emitter:
			col = colour(0.95, 0.35, 0.35, 1)
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
		for _, m := range a.msel {
			if m == i {
				dl.AddCircleV(p, 10, colour(0.95, 0.75, 0.25, 0.95), 0, 2)
				break
			}
		}
		if filtering {
			if a.nodeMatchesFilter(&a.Nodes[i]) {
				dl.AddCircleV(p, 12, colour(0.55, 0.95, 0.65, 0.9), 0, 2)
			} else {
				// Non-matches recede rather than vanish: the filter is a
				// highlighter, not a deletion preview.
				dl.AddCircleFilled(p, 6, colour(0.05, 0.06, 0.08, 0.55))
			}
		}
		// A ring, so a marker reads against both a dark hillshade and a light
		// street map without changing colour.
		dl.AddCircleFilled(p, 6, colour(0.05, 0.06, 0.08, 0.85))
		dl.AddCircleFilled(p, 4, col)

		if i != from && i != to {
			drawLabel(p, n.Name, false)
		}
	}

	// Said out loud. A map that quietly shows two hundred of four hundred names
	// is a map that has been lying about how many nodes are in the area.
	if dropped > 0 {
		note := fmt.Sprintf("%d of %d labels hidden - zoom in", dropped, labelled+dropped)
		at := imgui.NewVec2(origin.X+10, origin.Y+10)
		size := imgui.CalcTextSize(note)
		dl.AddRectFilledV(imgui.NewVec2(at.X-4, at.Y-2),
			imgui.NewVec2(at.X+size.X+4, at.Y+size.Y+2), colour(0.05, 0.06, 0.08, 0.7), 3, 0)
		dl.AddTextVec2V(at, colour(0.75, 0.78, 0.85, 1), note)
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

	// The scale and the attribution sit on their own strip, so both stay
	// readable whatever basemap is underneath. Attribution that cannot be read
	// is not attribution.
	attribution := a.attribution()
	stripW := px + 24
	if s := imgui.CalcTextSize(attribution); s.X+24 > stripW {
		stripW = s.X + 24
	}
	dl.AddRectFilledV(
		imgui.NewVec2(x-8, y-24),
		imgui.NewVec2(x+stripW, y+26),
		colour(0.05, 0.06, 0.08, 0.6), 4, 0)
	dl.AddTextVec2V(imgui.NewVec2(x, y-20), colour(0.95, 0.96, 1, 0.95), label)
	dl.AddTextVec2V(imgui.NewVec2(x, y+8), colour(0.9, 0.92, 0.96, 0.9), attribution)
}

// fetchVisibleTerrain downloads the tiles for what is on screen.
//
// In the background, with progress, and never as a side effect of panning.
// autoFetchTiles is the count below which terrain downloads without asking.
// Above it the cost is stated first: a few tiles is a moment, a country is not.
const autoFetchTiles = 60

// terrainEstimate is what the current boundary (or view) would cost to fetch.
func (a *App) terrainEstimate() (terrain.Estimate, bool) {
	fetcher, ok := a.Terrain.(interface {
		Estimate(south, north, west, east float64) terrain.Estimate
	})
	if !ok {
		return terrain.Estimate{}, false
	}
	south, north, west, east := a.view.Bounds()
	if region, _ := a.regionOrNil(); region != nil {
		south, north, west, east = region.Bounds()
	}
	return fetcher.Estimate(south, north, west, east), true
}

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
	// The boundary wins over the viewport. Someone who has said "this study
	// is Scotland" wants Scotland's terrain, not whatever the window happens to
	// show — and downloading by scrolling around is the workflow this replaces.
	south, north, west, east := a.view.Bounds()
	area := "the visible area"
	if region, _ := a.regionOrNil(); region != nil {
		south, north, west, east = region.Bounds()
		area = "the boundary area"
	}
	est := fetcher.Estimate(south, north, west, east)
	if est.ToFetch == 0 {
		a.status = "already have " + area
		return
	}
	a.status = fmt.Sprintf("downloading %d tiles for %s", est.ToFetch, area)

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

// startX and startY are where a drag began, in map coordinates.
//
// imgui reports the delta from the press, so the press position is the current
// mouse minus that delta — which is the point that decides whether this drag
// grabbed a node or the map, and it has to be answered once at the start rather
// than re-asked as the pointer moves off the node.
func startX(mx float64) float64 {
	return mx - float64(imgui.MouseDragDeltaV(imgui.MouseButtonLeft, 1).X)
}

func startY(my float64) float64 {
	return my - float64(imgui.MouseDragDeltaV(imgui.MouseButtonLeft, 1).Y)
}

// onGeometryChanged is what must happen when a node moves.
//
// Everything that depends on where things are: the link cache, the current
// path analysis, and the coverage overlay, which is now a picture of a network
// that no longer exists.
func (a *App) onGeometryChanged() {
	if a.eng != nil {
		a.eng.InvalidateLinks()
	}
	a.recompute()
	a.terrainDirty = true
}
