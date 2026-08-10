package ui

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/coverage"
	"github.com/A13xB0/meshcoresim/internal/gpu"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// coverageState is one node's coverage raster, drawn on the map.
// coverageMode is which question the overlay answers.
type coverageMode int

const (
	// covBest is the strongest server per cell — the map people actually want,
	// and the mode a single-node raster is also read as.
	covBest coverageMode = iota
	// covGap is cells nobody covers. The layer a planner acts on.
	covGap
	// covRedundancy is cells served by two or more: what survives a repeater
	// failing, which is the question asked after the first one does.
	covRedundancy
)

func (m coverageMode) String() string {
	switch m {
	case covBest:
		return "best server"
	case covGap:
		return "gaps"
	case covRedundancy:
		return "redundancy"
	default:
		return "one node"
	}
}

type coverageState struct {
	mode    coverageMode
	node    string
	running bool
	// cancel stops the compute goroutine between stations; the Jobs popover
	// holds the button.
	cancel context.CancelFunc
	// summary is what the combined layers found, in words.
	summary string
	// result crosses from the compute goroutine to the frame thread.
	result chan covResult
	tex    *imgui.TextureRef
	// The geographic box the texture belongs to, so the draw survives panning.
	south, north, west, east float64
	opacity                  float32
}

type covResult struct {
	img                      *image.RGBA
	south, north, west, east float64
	summary                  string
	err                      error
}

// covGrid is the raster resolution. 200x200 is 40,000 terrain profiles — a few
// seconds of work — and finer than the eye can use under map tiles.
const covGrid = 200

// covRangeKm is how far out the raster reaches. Past 60 km a LoRa link needs
// exceptional sites, and the raster's job is to show the shape of coverage,
// not to prove a negative at any distance.
const covRangeKm = 60

// startCoverage computes a coverage raster for one node, off-thread.
//
// The workbench had the whole coverage engine and never drew it — the CLI wrote
// PNGs while the map showed nothing. This is HopReach's central feature: the
// heatmap that says what a site actually reaches.
// startNetworkCoverage computes every transmitting node's raster and merges
// them, off-thread.
//
// The three layers the planner needs come from one computation: best server,
// gaps, and redundancy are three readings of the same merged grid, and
// computing them separately would let them disagree.
func (a *App) startNetworkCoverage(mode coverageMode) {
	if a.cov.running {
		a.status = "already computing coverage"
		return
	}
	var idx []int
	for i := range a.Nodes {
		if a.Nodes[i].Kind.Transmits() {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		a.status = "no transmitting nodes"
		return
	}
	// Bounded on the CPU, because the raster is per node and 600 of them is 24
	// million terrain profiles. Beyond the bound the GPU kernel takes over —
	// and where there is no usable GPU, the cap stays, said rather than
	// silently truncated (ADR-0025).
	const maxStations = 40
	useGPU := false
	if len(idx) > maxStations {
		if d, err := gpu.Open(); err == nil {
			d.Close() // reopened on the worker; this was the availability check
			useGPU = true
		} else {
			a.status = fmt.Sprintf("using the first %d of %d transmitters; more needs a GPU "+
				"and none is usable here (%v)", maxStations, len(idx), err)
			idx = idx[:maxStations]
		}
	}

	a.cov.mode, a.cov.running, a.cov.node = mode, true, "network"
	if a.cov.result == nil {
		a.cov.result = make(chan covResult, 1)
	}
	if a.cov.opacity == 0 {
		a.cov.opacity = 0.55
	}

	// One grid over every station, so the rasters can be merged at all.
	south, north, west, east := a.view.Bounds()
	if region, _ := a.regionOrNil(); region != nil {
		south, north, west, east = region.Bounds()
	}
	nodes := make([]scenario.Node, len(idx))
	for k, i := range idx {
		nodes[k] = a.Nodes[i]
	}
	terrain := a.Terrain
	res := a.cov.result
	ctx, cancel := context.WithCancel(context.Background())
	a.cov.cancel = cancel
	go func() {
		var dev *gpu.Device
		var grid coverage.HeightGrid
		if useGPU {
			d, err := gpu.Open()
			if err != nil {
				res <- covResult{err: fmt.Errorf("gpu: %w", err)}
				return
			}
			dev = d
			defer dev.Close()
			// The ground, rasterised once for every station. Twice the output
			// resolution, so profile sampling is not the resolution bottleneck.
			var known float64
			grid, known = coverage.RasteriseHeights(terrain, south, north, west, east,
				2*covGrid, 2*covGrid)
			if known < 0.5 {
				res <- covResult{err: fmt.Errorf(
					"terrain answers for only %.0f%% of the area - download tiles first", known*100)}
				return
			}
		}
		rasters := make([]*coverage.Raster, 0, len(nodes))
		for _, n := range nodes {
			if ctx.Err() != nil {
				res <- covResult{err: fmt.Errorf("cancelled")}
				return
			}
			r := &coverage.Raster{
				South: south, North: north, West: west, East: east,
				Width: covGrid, Height: covGrid,
				Cells:   make([]coverage.Cell, covGrid*covGrid),
				FreqMHz: n.Radio.CentreHz / 1e6,
			}
			fixed := coverage.Endpoint{
				Name: n.Name, Lat: n.Position.Lat, Lon: n.Position.Lon,
				HeightAGLm: n.HeightAGLm, TxPowerDBm: n.TxPowerDBm,
				SensitivityDBm: sensitivityFor(n.Radio),
				GainTowardsDBi: func(b, e float64) float64 { return n.Antenna.GainTowardsDBi(b, e) },
			}
			opts := coverage.Options{
				RemoteHeightAGLm: 1.5, RemoteTxPowerDBm: 20, RemoteGainDBi: 0,
				RemoteSensitivityDBm: sensitivityFor(n.Radio), ProfileStepM: 120,
			}
			if dev != nil {
				ground, ok := grid.At(n.Position.Lat, n.Position.Lon)
				if !ok {
					continue
				}
				losses, err := dev.CoverageGridLoss(grid, coverage.GridLossParams{
					StLat: n.Position.Lat, StLon: n.Position.Lon,
					StAltM:  ground + n.HeightAGLm,
					RasterW: covGrid, RasterH: covGrid,
					South: south, North: north, West: west, East: east,
					RemoteHeightM: 1.5, FreqMHz: n.Radio.CentreHz / 1e6, Steps: 200,
				})
				if err != nil {
					res <- covResult{err: fmt.Errorf("gpu: %w", err)}
					return
				}
				if err := coverage.ComputeFromLosses(fixed, grid, losses, r, opts); err != nil {
					continue
				}
				rasters = append(rasters, r)
				continue
			}
			if err := coverage.Compute(fixed, terrain, r, opts); err != nil {
				continue
			}
			rasters = append(rasters, r)
		}
		if len(rasters) == 0 {
			res <- covResult{err: fmt.Errorf("no terrain covers this area")}
			return
		}
		c, err := coverage.Combine(rasters)
		if err != nil {
			res <- covResult{err: err}
			return
		}
		gaps, known := c.GapCells()
		res <- covResult{
			img:   combinedImage(c, mode),
			south: south, north: north, west: west, east: east,
			summary: fmt.Sprintf("%d stations . %.0f%% covered . %d cells served by nobody . "+
				"%.0f%% of covered cells have a second server",
				len(rasters), 100*float64(known-gaps)/float64(max(known, 1)), gaps,
				100*c.Redundancy()),
		}
	}()
}

// combinedImage paints whichever layer was asked for.
//
// Different palettes on purpose. Gaps are what a planner acts on and must not
// be mistaken for coverage; proposed and real, present and absent, never share
// a colour ramp.
func combinedImage(c *coverage.Combined, mode coverageMode) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, c.Width, c.Height))
	for y := 0; y < c.Height; y++ {
		for x := 0; x < c.Width; x++ {
			i := y*c.Width + x
			serving := c.ServingCount[i]
			switch mode {
			case covGap:
				// Only the holes are painted. Everything else stays clear, so
				// the eye goes where the work is.
				if serving == 0 && !c.Cells[i].NoData {
					img.SetRGBA(x, y, color.RGBA{200, 60, 70, 150})
				}
			case covRedundancy:
				switch {
				case serving >= 3:
					img.SetRGBA(x, y, color.RGBA{60, 160, 90, 150})
				case serving == 2:
					img.SetRGBA(x, y, color.RGBA{120, 170, 70, 140})
				case serving == 1:
					// One server is coverage today and a hole the day it fails.
					img.SetRGBA(x, y, color.RGBA{225, 165, 60, 130})
				}
			default: // best server
				if serving == 0 {
					continue
				}
				m := c.BestMarginDB[i]
				t := math.Min(1, math.Max(0, m/20))
				img.SetRGBA(x, y, color.RGBA{
					uint8(230 - 130*t), uint8(140 + 80*t), 60, 165})
			}
		}
	}
	return img
}

func (a *App) startCoverage(i int) {
	if i < 0 || i >= len(a.Nodes) {
		return
	}
	n := a.Nodes[i]
	if !n.Kind.Transmits() {
		a.status = "an SDR observer transmits nothing, so it has no coverage"
		return
	}
	if a.cov.running {
		a.status = "already computing coverage for " + a.cov.node
		return
	}
	a.cov.node = n.Name
	a.cov.running = true
	if a.cov.result == nil {
		a.cov.result = make(chan covResult, 1)
	}
	if a.cov.opacity == 0 {
		a.cov.opacity = 0.55
	}
	a.status = "computing coverage from " + n.Name + "..."

	dLat := covRangeKm / 111.32
	dLon := covRangeKm / (111.32 * math.Cos(n.Position.Lat*math.Pi/180))
	r := &coverage.Raster{
		South: n.Position.Lat - dLat, North: n.Position.Lat + dLat,
		West: n.Position.Lon - dLon, East: n.Position.Lon + dLon,
		Width: covGrid, Height: covGrid,
		Cells:   make([]coverage.Cell, covGrid*covGrid),
		FreqMHz: n.Radio.CentreHz / 1e6,
	}
	fixed := coverage.Endpoint{
		Name: n.Name, Lat: n.Position.Lat, Lon: n.Position.Lon,
		HeightAGLm: n.HeightAGLm, TxPowerDBm: n.TxPowerDBm,
		SensitivityDBm: sensitivityFor(n.Radio),
		GainTowardsDBi: func(b, e float64) float64 { return n.Antenna.GainTowardsDBi(b, e) },
	}
	// The remote is a person with a handheld: 1.5 m, modest power, a dipole.
	// The same assumption the planning tools make, so the two agree.
	opts := coverage.Options{
		RemoteHeightAGLm: 1.5, RemoteTxPowerDBm: 20, RemoteGainDBi: 0,
		RemoteSensitivityDBm: sensitivityFor(n.Radio), ProfileStepM: 90,
	}

	terrain := a.Terrain
	res := a.cov.result
	ctx, cancel := context.WithCancel(context.Background())
	a.cov.cancel = cancel
	go func() {
		if err := coverage.Compute(fixed, terrain, r, opts); err != nil {
			res <- covResult{err: err}
			return
		}
		if ctx.Err() != nil {
			res <- covResult{err: fmt.Errorf("cancelled")}
			return
		}
		res <- covResult{
			img:   rasterImage(r),
			south: r.South, north: r.North, west: r.West, east: r.East,
		}
	}()
}

// rasterImage colours a raster HopReach's way: orange where marginal through
// green where solid, a distinct blue-grey where only one direction closes, and
// transparent where nothing does — the map should show through everywhere the
// answer is "no".
func rasterImage(r *coverage.Raster) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, r.Width, r.Height))
	for y := 0; y < r.Height; y++ {
		for x := 0; x < r.Width; x++ {
			c := r.At(x, y)
			switch {
			case c.NoData || (!c.Workable() && !c.OneWay()):
				// Transparent. NoData and no-coverage draw the same because the
				// overlay cannot carry the distinction legibly; the inspector
				// can, when a cell is asked about.
			case c.OneWay():
				img.SetRGBA(x, y, color.RGBA{110, 140, 200, 150})
			default:
				// Margin in the weaker direction drives the colour ramp.
				m := math.Min(c.OutboundMarginDB, c.InboundMarginDB)
				t := math.Min(1, m/20) // 0 dB orange -> 20+ dB green
				img.SetRGBA(x, y, color.RGBA{
					uint8(230 - 130*t), uint8(140 + 80*t), 60, 165,
				})
			}
		}
	}
	return img
}

// sensitivityFor is the demodulator floor for a radio config, from the Semtech
// SX1262 table the rest of the project uses.
func sensitivityFor(r scenario.RadioConfig) float64 {
	// -174 dBm/Hz thermal noise + bandwidth + 6 dB NF + per-SF SNR floor.
	snrFloor := map[int]float64{7: -7.5, 8: -10, 9: -12.5, 10: -15, 11: -17.5, 12: -20}
	f, ok := snrFloor[r.SpreadFactor]
	if !ok {
		f = -10
	}
	return -174 + 10*math.Log10(r.BandwidthHz) + 6 + f
}

// drawCoverage paints the raster under the nodes, and collects a finished
// computation if one is waiting.
func (a *App) drawCoverage(origin imgui.Vec2, w, h float32) {
	if a.cov.result != nil {
		select {
		case res := <-a.cov.result:
			a.cov.running = false
			a.cov.cancel = nil
			if res.err != nil {
				a.status = "coverage: " + res.err.Error()
			} else {
				tex := a.backend.CreateTextureRgba(res.img, res.img.Bounds().Dx(), res.img.Bounds().Dy())
				a.cov.tex = &tex
				a.cov.south, a.cov.north = res.south, res.north
				a.cov.west, a.cov.east = res.west, res.east
				a.cov.summary = res.summary
				if res.summary != "" {
					a.status = res.summary
				} else {
					a.status = fmt.Sprintf("coverage from %s", a.cov.node)
				}
			}
		default:
		}
	}
	if a.cov.tex == nil {
		return
	}

	x0, y0 := a.view.LatLonToScreen(a.cov.north, a.cov.west)
	x1, y1 := a.view.LatLonToScreen(a.cov.south, a.cov.east)
	dl := imgui.WindowDrawList()
	dl.PushClipRectV(origin, imgui.NewVec2(origin.X+w, origin.Y+h), true)
	alpha := uint32(a.cov.opacity*255) << 24
	dl.AddImageV(*a.cov.tex,
		imgui.NewVec2(origin.X+float32(x0), origin.Y+float32(y0)),
		imgui.NewVec2(origin.X+float32(x1), origin.Y+float32(y1)),
		imgui.NewVec2(0, 0), imgui.NewVec2(1, 1), alpha|0x00FFFFFF)
	dl.PopClipRect()

	// The overlay's own controls, on the overlay — after the raster so they
	// draw above it. Clearing used to live only in a right-click menu, which
	// is where features go to not be found.
	imgui.SetCursorScreenPos(imgui.NewVec2(origin.X+10, origin.Y+h-72))
	if imgui.BeginChildStrV("##covbadge", imgui.NewVec2(430, 62), 0, imgui.WindowFlagsNoScrollbar) {
		imgui.Text("coverage: " + a.cov.node + " (" + a.cov.mode.String() + ")")
		if a.cov.summary != "" {
			imgui.TextDisabled(a.cov.summary)
		}
		imgui.SetNextItemWidth(170)
		imgui.SliderFloat("##covop", &a.cov.opacity, 0.1, 1)
		imgui.SameLine()
		if imgui.Button("close") {
			a.clearCoverage()
		}
	}
	imgui.EndChild()
}

// clearCoverage drops the overlay.
func (a *App) clearCoverage() {
	a.cov.tex = nil
	a.cov.node = ""
}
