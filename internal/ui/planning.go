package ui

import (
	"fmt"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/antenna"
	"github.com/A13xB0/meshcoresim/internal/coverage"
	"github.com/A13xB0/meshcoresim/internal/planning"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// planState is the Planning window: what has been asked for, and the answer.
type planState struct {
	mastM    float32
	maxNew   int32
	routes   []planning.Route
	chosen   int
	places   []planning.Placement
	baseline float64
	status   string
}

// engineChecker answers "does this link work" with the same physics the
// simulation uses.
//
// The planner and the channel must not disagree: a route the planner proposes
// and the simulation then refuses is worse than no planner at all.
type engineChecker struct {
	app *App
}

func (c engineChecker) Works(a, b planning.Site) bool {
	fixed := coverage.Endpoint{
		Name: a.Name, Lat: a.Lat, Lon: a.Lon, HeightAGLm: a.HeightAGLm,
		TxPowerDBm: c.app.planTxDBm(), SensitivityDBm: c.app.planSensitivity(),
		GainTowardsDBi: func(float64, float64) float64 { return 6 },
	}
	r := &coverage.Raster{
		South: b.Lat, North: b.Lat, West: b.Lon, East: b.Lon,
		Width: 1, Height: 1, Cells: make([]coverage.Cell, 1),
		FreqMHz: c.app.freqMHz,
	}
	opts := coverage.Options{
		RemoteHeightAGLm: b.HeightAGLm, RemoteTxPowerDBm: c.app.planTxDBm(),
		RemoteGainDBi: 6, RemoteSensitivityDBm: c.app.planSensitivity(),
		ProfileStepM: 90,
	}
	if err := coverage.Compute(fixed, c.app.Terrain, r, opts); err != nil {
		return false
	}
	// Both directions, always. A link that closes one way is not a link, and
	// treating it as one is how a planned route fails in the field.
	return r.At(0, 0).Workable()
}

func (a *App) planTxDBm() float64 { return 22 }

func (a *App) planSensitivity() float64 {
	return sensitivityFor(scenario.RadioConfig{
		BandwidthHz: 62500, SpreadFactor: 8,
	})
}

func (a *App) drawPlanningBody() {
	p := &a.plan
	if p.mastM == 0 {
		p.mastM = 10
	}
	if p.maxNew == 0 {
		p.maxNew = 4
	}
	numF32("new mast height m", &p.mastM, 3, 100, "%.0f")
	imgui.SetNextItemWidth(110)
	imgui.InputInt("most new sites", &p.maxNew)
	if p.maxNew < 1 {
		p.maxNew = 1
	}

	imgui.SeparatorText("Coverage")
	if imgui.Button("from selected") {
		if i, _ := a.Link(); i >= 0 {
			a.startCoverage(i)
		} else {
			a.status = "select a node first"
		}
	}
	imgui.SameLine()
	if imgui.Button("best server") {
		a.startNetworkCoverage(covBest)
	}
	imgui.SameLine()
	if imgui.Button("gaps") {
		a.startNetworkCoverage(covGap)
	}
	imgui.SameLine()
	if imgui.Button("redundancy") {
		a.startNetworkCoverage(covRedundancy)
	}
	if a.cov.tex != nil {
		imgui.SetNextItemWidth(140)
		imgui.SliderFloat("opacity", &a.cov.opacity, 0.1, 1)
		imgui.SameLine()
		if imgui.Button("clear") {
			a.clearCoverage()
		}
		if a.cov.summary != "" {
			textDim(a.cov.summary)
		}
	} else if a.cov.running {
		textDim("computing...")
	} else {
		textDim("for a person with a handheld at 1.5 m")
	}

	imgui.SeparatorText("Connect two repeaters")
	from, to := a.Link()
	if from < 0 || to < 0 {
		textDim("select two nodes on the map (click, then ctrl-click)")
	} else {
		imgui.Text(fmt.Sprintf("%s  ->  %s", a.Nodes[from].Name, a.Nodes[to].Name))
		if imgui.Button("find the fewest new sites") {
			a.runBridge(from, to)
		}
	}
	if p.status != "" {
		textWrap(p.status)
	}

	for i, r := range p.routes {
		label := fmt.Sprintf("%d new site(s), longest hop %.1f km##r%d", r.NewSites, r.LongestHopKm, i)
		sel := p.chosen == i
		if imgui.SelectableBoolPtr(label, &sel) {
			p.chosen = i
		}
		if p.chosen == i {
			for _, s := range r.Sites {
				tag := "existing"
				if !s.Existing && s.Name != "" {
					tag = "NEW"
				}
				textDim(fmt.Sprintf("    %-10s %.5f, %.5f", tag, s.Lat, s.Lon))
			}
			if imgui.Button(fmt.Sprintf("place these sites##p%d", i)) {
				a.placeRoute(r)
			}
		}
	}

	imgui.SeparatorText("Cover an area")
	textDim("uses the boundary from the Boundary window")
	if imgui.Button("place sites for maximum coverage") {
		a.runCover()
	}
	for i, pl := range p.places {
		imgui.Text(fmt.Sprintf("%d. %.5f, %.5f  -  +%d cells, %.0f%% covered",
			i+1, pl.Site.Lat, pl.Site.Lon, pl.NewCellsCovered, pl.CoverageAfterPct))
	}
	if len(p.places) > 0 {
		textDim(fmt.Sprintf("before: %.0f%% covered", p.baseline))
		if imgui.Button("place all of these") {
			for _, pl := range p.places {
				a.placeSite(pl.Site)
			}
			a.buildEngine()
		}
	}
}

func (a *App) runBridge(from, to int) {
	p := &a.plan
	p.routes, p.status = nil, "searching..."

	site := func(n scenario.Node) planning.Site {
		return planning.Site{
			Lat: n.Position.Lat, Lon: n.Position.Lon,
			HeightAGLm: n.HeightAGLm, Name: n.Name, Existing: true,
		}
	}
	var existing []planning.Site
	for i := range a.Nodes {
		if i != from && i != to && a.Nodes[i].Kind.Transmits() {
			existing = append(existing, site(a.Nodes[i]))
		}
	}
	routes, err := planning.Bridge(site(a.Nodes[from]), site(a.Nodes[to]), a.Terrain,
		engineChecker{a}, planning.BridgeOptions{
			Existing: existing, MastHeightM: float64(p.mastM),
			MaxNew: int(p.maxNew), CandidateStep: 0.02, Alternatives: 3,
		})
	if err != nil {
		p.status = err.Error()
		return
	}
	p.routes, p.chosen, p.status = routes, 0, fmt.Sprintf("%d route(s)", len(routes))
}

func (a *App) runCover() {
	p := &a.plan
	region, _ := a.regionOrNil()
	if region == nil {
		p.status = "choose an area in the Boundary window first"
		return
	}
	// The first ring of the first boundary: covering a multipolygon nation is
	// not a question this tool answers well, and pretending otherwise would
	// place sites in the sea between islands.
	if len(region.Boundaries) == 0 || len(region.Boundaries[0].Rings) == 0 {
		p.status = "that boundary has no polygon"
		return
	}
	var area []planning.LatLon
	for _, pt := range region.Boundaries[0].Rings[0] {
		area = append(area, planning.LatLon{Lat: pt.Lat, Lon: pt.Lon})
	}

	var existing []planning.Site
	for i := range a.Nodes {
		if a.Nodes[i].Kind.Transmits() {
			existing = append(existing, planning.Site{
				Lat: a.Nodes[i].Position.Lat, Lon: a.Nodes[i].Position.Lon,
				HeightAGLm: a.Nodes[i].HeightAGLm, Existing: true,
			})
		}
	}
	opts := planning.CoverOptions{
		Existing: existing, MastHeightM: float64(p.mastM), MaxNew: int(p.maxNew),
		SampleStep: 0.02, CandidateStep: 0.03,
	}
	base, err := planning.BaselineCoverage(area, a.Terrain, engineChecker{a}, opts)
	if err != nil {
		p.status = err.Error()
		return
	}
	places, err := planning.CoverArea(area, a.Terrain, engineChecker{a}, opts)
	if err != nil {
		p.status = err.Error()
		return
	}
	p.baseline, p.places = base, places
	p.status = fmt.Sprintf("%d site(s) proposed", len(places))
}

// placeRoute adds a route's proposed sites as real nodes.
func (a *App) placeRoute(r planning.Route) {
	for _, s := range r.Sites {
		if s.Existing {
			continue
		}
		a.placeSite(s)
	}
	a.buildEngine()
}

func (a *App) placeSite(s planning.Site) {
	board, err := scenario.BoardByName(a.placeBoard)
	if err != nil {
		a.status = err.Error()
		return
	}
	n := scenario.Node{
		Name:     a.uniqueName("proposed"),
		Kind:     scenario.SimpleRepeater,
		Position: scenario.LatLon{Lat: s.Lat, Lon: s.Lon},
		// The mast height the search assumed, not a default: a proposed site
		// placed at a different height is not the site that was proposed.
		HeightAGLm:    s.HeightAGLm,
		TxPowerDBm:    board.MaxTxDBm,
		NoiseFigureDB: board.NoiseFigureDB,
		Antenna: antenna.Mounted{
			Pattern:      antenna.Collinear{GainDBiPeak: board.AntennaDBi + 4},
			Polarisation: "vertical", FeedlineDB: board.FeedlineDB,
		},
		Radio: scenario.RadioConfig{
			CentreHz: a.freqMHz * 1e6, BandwidthHz: 62500, SpreadFactor: 8, CodingRate: 4,
		},
	}
	a.Nodes = append(a.Nodes, n)
}
