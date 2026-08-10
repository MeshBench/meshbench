package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/boundary"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// boundaryState is the Boundary window: named areas in, a region out.
type boundaryState struct {
	query     string
	searching bool
	results   []boundary.Found
	err       string
	found     chan boundarySearch

	// chosen is the table: the union of these areas is the active region.
	chosen   []boundary.Found
	marginKm float32
}

type boundarySearch struct {
	results []boundary.Found
	err     error
}

func (a *App) drawBoundaryBody() {
	b := &a.bnd

	imgui.SetNextItemWidth(-90)
	entered := imgui.InputTextWithHint("##bquery", "a place name: Scotland, Ireland, Fife...",
		&b.query, imgui.InputTextFlagsEnterReturnsTrue, nil)
	imgui.SameLine()
	if (imgui.Button("search") || entered) && b.query != "" && !b.searching {
		a.startBoundarySearch(b.query)
	}

	if b.found != nil {
		select {
		case res := <-b.found:
			b.searching = false
			b.err = ""
			if res.err != nil {
				b.err = res.err.Error()
				b.results = nil
			} else {
				b.results = res.results
			}
		default:
		}
	}

	switch {
	case b.searching:
		imgui.TextDisabled("searching...")
	case b.err != "":
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
		imgui.TextWrapped(b.err)
		imgui.PopStyleColor()
	}

	// Candidates. Shown with their full display name because "Perth" is a
	// Scottish city, an Australian one, and a bus stop, and the short label
	// alone lets someone filter their network to the wrong hemisphere.
	for i, f := range b.results {
		if imgui.ButtonV(fmt.Sprintf("add##%d", i), imgui.NewVec2(40, 0)) {
			b.chosen = append(b.chosen, f)
			b.results = nil
			break
		}
		imgui.SameLine()
		imgui.Text(f.DisplayName)
		imgui.SameLine()
		imgui.TextDisabled("(" + f.Kind + ")")
	}

	// Inferred from where the nodes actually are, rather than typed.
	//
	// A loaded deployment already says which country it is in; asking the
	// operator to name it again is asking them to repeat what the data knows.
	if len(a.Nodes) > 0 {
		if imgui.Button("infer from the loaded network") {
			a.inferAreaFromNodes()
		}
		if imgui.IsItemHovered() {
			imgui.SetTooltip("Searches for the place the loaded nodes sit in, and adds it.\n" +
				"Uses the median position, so a handful of far-flung nodes do not\n" +
				"decide which country the study is about.")
		}
	}

	imgui.SetNextItemWidth(-1)
	imgui.InputTextWithHint("##bndpath", "...or a GeoJSON file path", &a.boundaryPath, 0, nil)

	imgui.SeparatorText("Chosen areas")
	if len(b.chosen) == 0 {
		imgui.TextDisabled("none yet - the region is the union of what is added here")
	}
	if imgui.BeginTableV("##chosen", 3, imgui.TableFlagsBorders|imgui.TableFlagsRowBg,
		imgui.NewVec2(0, 0), 0) {
		for i := 0; i < len(b.chosen); i++ {
			f := b.chosen[i]
			imgui.TableNextRow()
			imgui.TableSetColumnIndex(0)
			imgui.Text(f.Name)
			imgui.TableSetColumnIndex(1)
			parts := 0
			for _, bd := range f.Boundaries {
				parts += len(bd.Rings)
			}
			imgui.TextDisabled(fmt.Sprintf("%d polygon(s)", parts))
			imgui.TableSetColumnIndex(2)
			if imgui.SmallButton(fmt.Sprintf("remove##%d", i)) {
				b.chosen = append(b.chosen[:i], b.chosen[i+1:]...)
				i--
			}
		}
		imgui.EndTable()
	}

	if len(b.chosen) == 0 {
		return
	}

	if b.marginKm == 0 {
		b.marginKm = scenario.DefaultMarginKm
	}
	imgui.SetNextItemWidth(180)
	imgui.SliderFloat("RF margin km", &b.marginKm, 0, 50)
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Nodes outside the area but within this margin are kept:\n" +
			"a repeater just over the border still relays into the area.\n" +
			"Zero means a hard cut at the line.")
	}

	if imgui.Button("delete nodes outside these areas") {
		a.pruneOutside()
	}
	imgui.SameLine()
	imgui.TextDisabled("also filters every future load")

	// The terrain for the area, estimated before it is fetched.
	//
	// hamreach's HAM-18 pattern: a naive fetch there pulled 729 tiles and took
	// 64 seconds with no warning. Below the threshold this just goes; above it,
	// the cost is stated and the operator decides.
	imgui.Spacing()
	if est, ok := a.terrainEstimate(); ok {
		switch {
		case est.ToFetch == 0:
			imgui.TextDisabled("terrain for this area is already downloaded")
		case est.ToFetch <= autoFetchTiles:
			if imgui.Button(fmt.Sprintf("download terrain (%d tiles)", est.ToFetch)) {
				a.fetchVisibleTerrain()
			}
		default:
			imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
			imgui.TextWrapped(fmt.Sprintf("%d tiles, about %d MB", est.ToFetch, est.BytesRough/(1<<20)))
			imgui.PopStyleColor()
			if imgui.Button("download it anyway") {
				a.fetchVisibleTerrain()
			}
		}
	}
}

func (a *App) startBoundarySearch(query string) {
	b := &a.bnd
	b.searching = true
	if b.found == nil {
		b.found = make(chan boundarySearch, 1)
	}
	client := &boundary.Client{CacheDir: boundaryCacheDir()}
	ch := b.found
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		results, err := client.Search(ctx, query)
		ch <- boundarySearch{results: results, err: err}
	}()
}

func boundaryCacheDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return "boundaries"
	}
	return filepath.Join(base, "meshcoresim", "boundaries")
}

// chosenRegion is the region the Boundary window has built, or nil.
func (a *App) chosenRegion() *scenario.Region {
	if len(a.bnd.chosen) == 0 {
		return nil
	}
	var bounds []scenario.Boundary
	for _, f := range a.bnd.chosen {
		bounds = append(bounds, f.Boundaries...)
	}
	return &scenario.Region{Boundaries: bounds, MarginKm: float64(a.bnd.marginKm)}
}

// drawBoundaryOutline draws the chosen areas on the map, so "did I pick the
// right Scotland" is answered by looking rather than by counting nodes after a
// prune has already deleted them.
func (a *App) drawBoundaryOutline(origin imgui.Vec2, w, h float32) {
	if !a.layers.region {
		return
	}
	region := a.chosenRegion()
	if region == nil {
		return
	}
	dl := imgui.WindowDrawList()
	dl.PushClipRectV(origin, imgui.NewVec2(origin.X+w, origin.Y+h), true)
	defer dl.PopClipRect()

	col := imgui.ColorU32Vec4(imgui.NewVec4(0.55, 0.65, 0.95, 0.9))
	for _, bd := range region.Boundaries {
		for _, ring := range bd.Rings {
			drawRing(dl, a.view, origin, ring, col)
		}
	}
}

func drawRing(dl *imgui.DrawList, v MapView, origin imgui.Vec2, ring scenario.Ring, col uint32) {
	if len(ring) < 3 {
		return
	}
	// Stride caps the segment count. A national coastline at full detail is
	// tens of thousands of points, and the outline is orientation, not survey.
	stride := len(ring)/1500 + 1
	var prev imgui.Vec2
	first := true
	step := func(p scenario.LatLon) {
		x, y := v.LatLonToScreen(p.Lat, p.Lon)
		pt := imgui.NewVec2(origin.X+float32(x), origin.Y+float32(y))
		if !first {
			dl.AddLineArgs(prev, pt, col, 1.5)
		}
		prev, first = pt, false
	}
	for i := 0; i < len(ring); i += stride {
		step(ring[i])
	}
	step(ring[0]) // close it
}

// inferAreaFromNodes works out where the loaded network is and adds that area.
//
// From the median position rather than the mean or the bounding box centre: a
// deployment with three nodes in Spain and three hundred in Scotland is a
// Scottish deployment, and both of the other measures would put its centre in
// the Bay of Biscay.
func (a *App) inferAreaFromNodes() {
	if len(a.Nodes) == 0 {
		return
	}
	lats := make([]float64, 0, len(a.Nodes))
	lons := make([]float64, 0, len(a.Nodes))
	for i := range a.Nodes {
		p := a.Nodes[i].Position
		if p.Lat == 0 && p.Lon == 0 {
			continue // a node with no position says nothing about where this is
		}
		lats = append(lats, p.Lat)
		lons = append(lons, p.Lon)
	}
	if len(lats) == 0 {
		a.bnd.err = "no node has a position, so there is nothing to infer from"
		return
	}
	sort.Float64s(lats)
	sort.Float64s(lons)
	lat, lon := lats[len(lats)/2], lons[len(lons)/2]

	a.bnd.searching = true
	if a.bnd.found == nil {
		a.bnd.found = make(chan boundarySearch, 1)
	}
	client := &boundary.Client{CacheDir: boundaryCacheDir()}
	ch := a.bnd.found
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		results, err := client.ReverseSearch(ctx, lat, lon)
		ch <- boundarySearch{results: results, err: err}
	}()
}
