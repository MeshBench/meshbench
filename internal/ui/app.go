// Package ui is the desktop application.
//
// A native window, not a browser and not a server with a front end pointed at
// it. The whole simulator runs in this process; there is nothing to deploy and
// nothing listening.
//
// The shell owns layout and input and nothing else. Every number it draws comes
// from the engine packages, and it deliberately holds no model of its own — a UI
// that keeps its own copy of a link budget is a second implementation of the
// physics, and the two will disagree eventually.
package ui

import (
	"fmt"
	"image"

	"github.com/AllenDang/cimgui-go/backend"
	"github.com/AllenDang/cimgui-go/backend/glfwbackend"
	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/antenna"
	"github.com/A13xB0/meshcoresim/internal/pathview"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// Terrain is the elevation source. An interface so the application can be run
// against a cache, a live tile store, or nothing at all — and so the panels can
// be tested without a window.
type Terrain interface {
	ElevationM(lat, lon float64) (float64, bool)
}

// CachedTerrain is a terrain source that can answer without blocking.
//
// Drawing uses it where it exists. Everything that computes a *result* — a link
// budget, a profile — uses ElevationM and is allowed to wait, because an answer
// built from gaps would be wrong rather than merely incomplete.
type CachedTerrain interface {
	ElevationCachedM(lat, lon float64) (float64, bool)
}

// App is the running application.
type App struct {
	Terrain Terrain

	// Nodes is the scenario. The UI edits this and nothing else; everything
	// derived is recomputed rather than stored, so there is no second copy to
	// drift.
	Nodes []scenario.Node

	// selected indexes Nodes; -1 for none. Two selections make a link, which is
	// the only way to ask for a path view.
	selected int
	linkTo   int

	freqMHz float64
	cut     *pathview.CutThrough
	cutErr  string

	// view is the map. It is the main object on screen and everything else is
	// arranged around it.
	view MapView
	tool Tool
	// placeBoard is the hardware a newly placed node gets. Named rather than
	// defaulted silently, for the same reason the importer refuses without one.
	placeBoard string

	terrainTex   *imgui.TextureRef
	terrainDirty bool
	rendering    bool
	pending      chan *image.RGBA
	terrainW     int
	terrainH     int
	dragged      bool
	status       string

	backend backend.Backend[glfwbackend.GLFWWindowFlags]
}

// New prepares an application over a terrain source.
func New(t Terrain) *App {
	a := &App{
		Terrain:    t,
		selected:   -1,
		linkTo:     -1,
		freqMHz:    869.525,
		Nodes:      demoScenario(),
		placeBoard: "RAK4631",
		pending:    make(chan *image.RGBA, 1),
	}
	// Open on a worked example rather than an empty panel. The first thing a
	// user sees should be an answer they can interrogate, not an instruction to
	// go and produce one — and the demo path is chosen because it fails for a
	// reason the picture makes obvious.
	a.selectFirstLink()
	return a
}

// selectFirstLink picks the first two nodes that can actually form a link.
func (a *App) selectFirstLink() {
	var ends []int
	for i, n := range a.Nodes {
		if n.Kind.Transmits() {
			ends = append(ends, i)
			if len(ends) == 2 {
				break
			}
		}
	}
	if len(ends) < 2 {
		return
	}
	a.SelectNode(ends[0], false)
	a.SelectNode(ends[1], true)
}

// demoScenario is what opens on a first run.
//
// Real places, real board profiles and a link that genuinely fails, because an
// empty window teaches nothing and a demo that always works teaches the wrong
// thing. The Ben Vrackie to Perth path is blocked by the hill the repeater is
// standing on, which is the most instructive first thing to see.
func demoScenario() []scenario.Node {
	rak, _ := scenario.BoardByName("RAK4631")
	xiao, _ := scenario.BoardByName("Xiao_nRF52840")
	radio := scenario.RadioConfig{CentreHz: 869.525e6, BandwidthHz: 250e3, SpreadFactor: 10, CodingRate: 1}

	// Antennas come from the board profile, not from a convenient constant. The
	// gap between a repeater's collinear and a handheld's chip antenna is most
	// of the asymmetry in every link these nodes form, so a demo that gives
	// both the same antenna hides the thing worth seeing.
	mast := func(b scenario.Board) antenna.Mounted {
		return antenna.Mounted{
			Pattern:      antenna.Collinear{GainDBiPeak: b.AntennaDBi + 4},
			Polarisation: "vertical", FeedlineDB: b.FeedlineDB,
		}
	}
	handheld := func(b scenario.Board) antenna.Mounted {
		return antenna.Mounted{
			Pattern:      antenna.Dipole{},
			Polarisation: "vertical", FeedlineDB: b.FeedlineDB - b.AntennaDBi,
		}
	}

	return []scenario.Node{
		{
			Name: "Ben Vrackie", Kind: scenario.SimpleRepeater,
			Position: scenario.LatLon{Lat: 56.7472, Lon: -3.7411}, HeightAGLm: 15,
			Antenna: mast(rak),
			Radio:   radio, TxPowerDBm: rak.MaxTxDBm, NoiseFigureDB: rak.NoiseFigureDB,
		},
		{
			Name: "Perth", Kind: scenario.Companion,
			Position: scenario.LatLon{Lat: 56.3950, Lon: -3.4308}, HeightAGLm: 2,
			Antenna: handheld(xiao),
			// A companion runs well below its chip's maximum: 14 dBm is a
			// common EU setting, and battery is why. Giving it the repeater's
			// 22 dBm makes both directions come out identical, which is
			// arithmetically right and hides the single thing this tool exists
			// to show.
			Radio: radio, TxPowerDBm: 14, NoiseFigureDB: xiao.NoiseFigureDB,
		},
		{
			Name: "Dunkeld", Kind: scenario.SimpleRepeater,
			Position: scenario.LatLon{Lat: 56.5646, Lon: -3.5876}, HeightAGLm: 12,
			Antenna: mast(rak),
			Radio:   radio, TxPowerDBm: rak.MaxTxDBm, NoiseFigureDB: rak.NoiseFigureDB,
		},
		{
			Name: "observer-1", Kind: scenario.SDRObserver,
			Position: scenario.LatLon{Lat: 56.6000, Lon: -3.6500}, HeightAGLm: 5,
			Antenna: mast(rak),
			Radio:   radio, NoiseFigureDB: 6,
		},
	}
}

// Run opens the window and does not return until it is closed.
func (a *App) Run(title string, w, h int) error {
	b, err := backend.CreateBackend(glfwbackend.NewGLFWBackend())
	if err != nil {
		return fmt.Errorf("ui: no window backend: %w", err)
	}
	a.backend = b
	b.SetBgColor(imgui.NewVec4(0.06, 0.07, 0.09, 1))
	b.CreateWindow(title, w, h)
	b.Run(a.frame)
	return nil
}

// frame draws one frame.
//
// The map is the main view and the panels are around it. That is not a style
// choice: a list of four nodes is fine and a list of four hundred is unusable,
// and the questions a workbench exists to answer are spatial.
func (a *App) frame() {
	vp := imgui.MainViewport()
	imgui.SetNextWindowPos(vp.Pos())
	imgui.SetNextWindowSize(vp.Size())

	flags := imgui.WindowFlagsNoTitleBar | imgui.WindowFlagsNoResize |
		imgui.WindowFlagsNoMove | imgui.WindowFlagsNoBringToFrontOnFocus |
		imgui.WindowFlagsNoNavFocus
	imgui.BeginV("##root", nil, flags)

	a.uploadPendingTerrain()
	a.drawHeader()
	imgui.Separator()
	a.drawToolbar()

	fill := imgui.ContentRegionAvail()
	const inspectorW = 300
	const bottomH = 250

	if imgui.BeginTableV("##layout", 2, imgui.TableFlagsResizable|imgui.TableFlagsBordersInnerV,
		imgui.NewVec2(0, fill.Y), 0) {
		imgui.TableSetupColumnV("map", imgui.TableColumnFlagsWidthStretch, 0, 0)
		imgui.TableSetupColumnV("inspector", imgui.TableColumnFlagsWidthFixed, inspectorW, 0)

		imgui.TableNextRow()
		imgui.TableSetColumnIndex(0)
		mapW := imgui.ContentRegionAvail().X
		a.drawMap(mapW, fill.Y-bottomH)
		imgui.Spacing()
		a.drawAnalysis()

		imgui.TableSetColumnIndex(1)
		a.drawInspector()
		imgui.EndTable()
	}

	imgui.End()
}

// drawToolbar is the palette: what a click on the map will do.
func (a *App) drawToolbar() {
	tools := []Tool{ToolSelect, ToolPlaceRepeater, ToolPlaceCompanion, ToolPlaceObserver, ToolPlaceCustom}
	for i, t := range tools {
		if i > 0 {
			imgui.SameLine()
		}
		active := a.tool == t
		if active {
			imgui.PushStyleColorVec4(imgui.ColButton, imgui.NewVec4(0.22, 0.42, 0.62, 1))
		}
		if imgui.Button(t.label()) {
			a.tool = t
		}
		if active {
			imgui.PopStyleColor()
		}
	}

	imgui.SameLineV(0, 24)
	if imgui.Button("fit") {
		a.view.FitTo(a.Nodes, a.view.Width, a.view.Height)
		a.terrainDirty = true
	}
	imgui.SameLine()
	imgui.TextDisabled(fmt.Sprintf("%d nodes  |  %.0f m/px", len(a.Nodes), a.view.MetresPerPixel))
	if a.status != "" {
		imgui.SameLineV(0, 24)
		imgui.TextDisabled(a.status)
	}
	imgui.Separator()
}

func (a *App) drawHeader() {
	imgui.Text("MeshcoreSim")
	imgui.SameLine()
	imgui.TextDisabled("- real firmware, real RF")

	// The honesty line is in the chrome, not in a help menu. CLAUDE.md requires
	// the simulator to say that it is kinder than the air, and something a user
	// has to go looking for does not count as saying it.
	imgui.SameLineV(0, 24)
	imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
	imgui.Text("Results are a best case: no multipath, bare-earth terrain, idealised demodulator.")
	imgui.PopStyleColor()
}
