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
	"context"
	"fmt"
	"image"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AllenDang/cimgui-go/backend"
	"github.com/AllenDang/cimgui-go/backend/glfwbackend"
	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/antenna"
	"github.com/A13xB0/meshcoresim/internal/basemap"
	"github.com/A13xB0/meshcoresim/internal/control"
	"github.com/A13xB0/meshcoresim/internal/engine"
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

	composite Composite
	// placeBoard is the hardware a newly placed node gets. Named rather than
	// defaulted silently, for the same reason the importer refuses without one.
	placeBoard string

	tiles      *tileCache
	bmStore    *basemap.Store
	fetchTiles bool

	terrainTex   *imgui.TextureRef
	terrainDirty bool
	// terrainView is the view the current hillshade was rendered for, and
	// pendingView the one being rendered now: a shade drawn at the wrong
	// view is a map whose hills lag behind its nodes.
	terrainView MapView
	pendingView MapView
	rendering   bool
	pending     chan *image.RGBA
	terrainW    int
	terrainH    int
	dragged     bool
	status      string

	fetchMu     sync.Mutex
	fetching    bool
	fetchStatus string

	// eng is the running simulation. Everything on the traffic tabs is a view
	// onto it, and nothing keeps its own copy of what happened.
	eng     *engine.Engine
	playing bool
	scrubMs uint32

	nodeFilter string
	// msel is the multi-selection (shift-click); indices into Nodes. Cleared
	// by any plain click and by anything that renumbers the node list.
	msel          []int
	boundaryPath  string
	saveName      string
	projName      string
	savedCache    []savedNet
	savedScanAt   time.Time
	savedDirty    bool
	confirmDelete string

	// Timeline state: the snapshot, the filter ticks, and the filtered index
	// they produce.
	evSnapshot   []engine.Event
	evShow       [5]bool
	evFilterInit bool
	evKey        evFilterKey
	evFiltered   []int

	// Right-click context: which node (or ground position) the menu is about.
	ctxNode        int
	ctxLat, ctxLon float64

	// Floating windows. Per-node windows are keyed by name so any number can be
	// open at once — watching three repeaters independently is the use case.
	nodeWindows   map[string]bool
	winNodesTable bool
	speed         float32
	stepDebt      float32

	// Link-matrix warming: cancel for the run in flight, and progress the
	// toolbar can read from any thread.
	warmCancel   context.CancelFunc
	warmDone     atomic.Int64
	warmTotal    atomic.Int64
	bnd          boundaryState
	fleet        fleetState
	cov          coverageState
	comp         companionState
	pkt          packetState
	cfg          configState
	winProvision bool
	winFirmware  bool
	confirmWipe  bool
	winPrefs     bool
	infer        inferState
	ab           abState
	val          validateState
	live         liveState
	energy       energyState
	// excessLossDB is the ADR-0015 calibration, applied to every engine build
	// until removed. Displayed wherever it is in force.
	excessLossDB float64
	// layerID is the chosen basemap, remembered across launches.
	layerID string
	// detach marks node windows to be pushed outside the main window on the
	// next frame, for a second monitor.
	detach map[string]bool
	// redock is the other direction: a window asked to come home; undock
	// floats it loose *inside* the main window; popped records which windows
	// are OS windows of their own, so the rest can be pinned in.
	redock map[string]bool
	undock map[string]bool
	popped map[string]bool

	// comps are open mini-companion sessions, one per node, each holding
	// that node's serial port; compUI is what their tab has typed into it.
	comps  map[string]*compSession
	compUI map[string]*compUIState

	// journal is every command this session has been driven with - the only
	// memory a driven workbench has of what was done to it.
	journal []map[string]any
	// confirmRestart arms the run-strip restart; viewName and confirmView
	// belong to the Views menu.
	confirmRestart bool
	viewName       string
	confirmView    string

	// Workspaces: which is active, whether its preset needs building, and the
	// panel registry the menu and layouts are generated from.
	ws        workspace
	wsForce   bool
	wsRebuild bool
	panelList []*panelSpec
	plan      planState
	sched     scheduleState
	tg        timeGraphState
	dragNode  int
	seed      uint64
	ctrl      *control.Server
	layers    mapLayers
	// neighboursOf is the node whose links are drawn, set from the map's
	// right-click menu rather than by selection.
	neighboursOf string
	wf           waterfallState
	// hashNames maps a MeshCore path hash to a node name where the run has
	// shown us which is which.
	hashNames map[byte]string
	// One scrollback per node, kept across tab switches: a repeater prints while
	// you are looking at something else, and losing that is losing the record of
	// what it did.
	consoles map[string]*consoleBuf

	// imp is the Import window — Scenario in, previewed first.
	imp importState

	// What firmware is published, for the per-node picker.
	fw *fwCatalogue

	// uiScale is the multiplier currently applied to the style and fonts;
	// pendingScale is a requested change, applied at the top of the next
	// frame — rescaling the style while half a frame's windows are already
	// laid out at the old sizes tears the layout for that frame.
	uiScale      float64
	pendingScale float64

	// noViewports means the platform cannot make panels their own OS windows
	// (native Wayland). The buttons explain rather than fail.
	noViewports bool
	// railRight is where the tool rail actually ended, measured each frame.
	railRight float32

	// UIScale multiplies every size in the UI. Zero means detect: the X11
	// content scale when it says anything, environment hints when it does not
	// — which is the usual case under XWayland, where the X side reports 1.0
	// while the monitor is scaled, and the whole UI renders miniature.
	UIScale float64

	backend backend.Backend[glfwbackend.GLFWWindowFlags]
}

// New prepares an application over a terrain source.
func New(t Terrain) *App {
	a := &App{
		Terrain:    t,
		selected:   -1,
		linkTo:     -1,
		freqMHz:    scenario.DefaultFreqMHz(),
		Nodes:      demoScenario(),
		placeBoard: "RAK4631",
		pending:    make(chan *image.RGBA, 1),
		tiles:      newTileCache(),
		fw:         &fwCatalogue{},
	}
	// Hillshade only by default. Every imagery layer here has terms that have
	// not been checked against how this application uses them, and a default
	// that quietly contacts a third party is not a default anyone chose.
	a.composite.ShadeMix = 0.55
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
	radio := scenario.DefaultRadio()

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
			// Not a place name: the demo's companion used to be called "Perth",
			// which collides with a real Perth repeater on any Scottish import
			// and makes "Perth is not a companion" a reasonable objection.
			Name: "handheld", Kind: scenario.Companion,
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

	// Vsync, and a frame cap under it.
	//
	// Without this the loop renders as fast as the machine allows and burns two
	// cores doing it — measured at 184% on four nodes and 186% on four hundred,
	// which is what proved the cost was the render loop rather than the
	// scenario. Nothing here animates faster than a person can pan a map.
	b.SetTargetFPS(60)

	b.SetBgColor(imgui.NewVec4(0.06, 0.07, 0.09, 1))
	// Two stages, because the platform cannot be asked before a window exists
	// (asking was a segfault): the flag and environment size the window, and
	// the platform's own answer joins in afterwards for the style.
	a.ensureConfig() // the saved ctrl+/- scale is part of scale resolution
	scale := a.configuredUIScale()
	b.CreateWindow(title, int(float64(w)*scale), int(float64(h)*scale))
	// Maximised: this is a workbench with a map, four panel groups and a
	// status bar, and none of that is better in a third of a screen. The flag
	// is set after the window exists, because GLFW applies it to the window
	// rather than to the hint.
	b.SetWindowFlags(glfwbackend.GLFWWindowFlagsMaximized, 1)
	if scale == 1 {
		if sx, _ := b.ContentScale(); sx > 1.01 {
			scale = float64(sx)
		}
	}
	if scale == 1 {
		// Nothing has said otherwise, so judge by the display. A 4K monitor
		// reporting no content scale (XWayland does exactly this) rendered
		// everything at a size meant for 1080p, which is what "cramped even on
		// a 4K" is: not a layout problem, a scale problem.
		if dw := imgui.MainViewport().Size().X; dw >= 3000 {
			scale = 1.5
		} else if dw >= 2200 {
			scale = 1.25
		}
	}
	loadFonts()
	applyTheme()
	a.uiScale = 1
	a.applyUIScale(scale)
	// First launch of a workspace builds its preset; every later launch loads
	// what the operator made of it.
	a.wsForce = true
	a.switchWorkspace(wsPlan)

	// Windows become their own OS window as soon as they leave the main one,
	// rather than only when dropped somewhere outside it.
	//
	// imgui's default is to merge a floating window back into the main viewport
	// whenever it overlaps — which makes dragging one to a second monitor
	// impossible the moment the main window is maximised, because there is no
	// "outside" left to drop it in. That is the whole of why this did not work.
	io := imgui.CurrentIO()
	io.SetConfigViewportsNoAutoMerge(true)
	// Under native Wayland the imgui backend refuses multi-viewport outright
	// (upstream #8587: the protocol forbids positioning windows globally).
	// Recorded so the pop-out buttons can say so instead of silently doing
	// nothing — the exact lie this feature kept telling.
	a.noViewports = io.BackendFlags()&imgui.BackendFlagsPlatformHasViewports == 0
	if a.noViewports {
		// Said out loud at startup. This state used to be reached silently by
		// every normal launch on a Wayland desktop, and the only symptom was
		// pop-out buttons that did nothing.
		a.status = "running on native Wayland: panels cannot leave this window " +
			"(the protocol forbids it). Launch without -wayland for pop-out."
	}
	// A detached window keeps its own decoration, so it can be moved and closed
	// with the window manager like any other.
	io.SetConfigViewportsNoDecoration(false)

	// The control socket exists only while the window does.
	a.startControl()
	defer func() {
		if a.ctrl != nil {
			_ = a.ctrl.Close()
		}
	}()
	// Closing the main window is quitting: every popped-out panel dies with
	// the process, and the firmware children are reaped rather than orphaned.
	defer func() {
		a.closeCompanions()
		if a.eng != nil {
			_ = a.eng.Close()
		}
	}()
	// Best-effort: a context that cannot vsync (software GL, some remote X
	// servers) still runs, just paced by SetTargetFPS alone. Refusing to launch
	// over a missing luxury would be the wrong trade.
	_ = b.SetSwapInterval(glfwbackend.GLFWSwapIntervalVsync)
	b.Run(a.frame)
	return nil
}

// frame draws one frame.
//
// The map is the main view and the panels are around it. That is not a style
// choice: a list of four nodes is fine and a list of four hundred is unusable,
// and the questions a workbench exists to answer are spatial.
func (a *App) frame() {
	// Anything an external client asked for runs here, on the only thread
	// allowed to touch the UI.
	if a.ctrl != nil {
		a.ctrl.Pump()
	}
	a.pumpUIScale()

	vp := imgui.MainViewport()
	statusH := imgui.FrameHeight() + 10
	imgui.SetNextWindowPos(vp.Pos())
	imgui.SetNextWindowSize(imgui.NewVec2(vp.Size().X, vp.Size().Y-statusH))
	// Pin the shell to the real OS window. With no-auto-merge, imgui was
	// giving this fullscreen host its *own* platform window stacked exactly
	// over the GLFW one - so the visible "main window" was an imposter: its
	// close button reached imgui rather than the app, and maximising it made
	// imgui and KWin fight over the geometry every frame, which is the
	// flicker. Pinned, the window the operator sees *is* the one whose close
	// quits.
	imgui.SetNextWindowViewport(vp.ID())

	// The host window carries the chrome — menu, honesty line, toolbar, run
	// strip — and a dockspace. Everything else is a dockable panel inside it.
	// NoDocking on the host itself, or panels dock into the chrome.
	flags := imgui.WindowFlagsNoTitleBar | imgui.WindowFlagsNoResize |
		imgui.WindowFlagsNoMove | imgui.WindowFlagsNoBringToFrontOnFocus |
		imgui.WindowFlagsNoNavFocus | imgui.WindowFlagsMenuBar |
		imgui.WindowFlagsNoDocking
	imgui.BeginV("##root", nil, flags)

	a.drawMenuBar()
	imgui.Separator()
	a.drawControlRow()
	imgui.Separator()

	dockID := dockspaceID()
	a.applyRedocks()
	if a.wsRebuild {
		a.wsRebuild = false
		a.buildWorkspace(dockID)
	}
	// The plain call: the V variant takes a *WindowClass whose nil handle the
	// binding dereferences, which is a crash dressed as an option.
	imgui.DockSpace(dockID)
	imgui.End()

	// The map is a dockable window like everything else — the central node of
	// every preset, but an operator who wants it floating on another monitor
	// entirely is not wrong.
	if imgui.BeginV("Map", nil, 0) {
		avail := imgui.ContentRegionAvail()
		a.drawMap(avail.X, avail.Y)
	}
	imgui.End()

	a.drawPanels()
	a.drawStatusBar()
	a.pumpLiveFeed()
	a.pumpCompanions()

	// Windows that are not dockable panels: they are modal-ish, per-node, or
	// their own top-level things.
	a.drawNodesTableWindow()
	a.drawFirmwareWindow()
	a.drawProvisionWindow()
	a.drawPrefsWindow()
	a.drawPacketWindow()
	a.drawNodeWindows()
}

// drawControlRow is the one instrument row: which view you are in, what the
// simulation is doing, and the facts that must never be hidden.
//
// It replaces two rows - a palette row and a run strip - because the palette
// belongs beside the map it acts on (drawToolRail) and the facts belong with
// the transport. Chrome that reflows is not chrome, so nothing here appears
// or disappears except by the operator's choice.
func (a *App) drawControlRow() {
	a.drawViewTabs()
	imgui.SameLineV(0, 18)
	a.drawRunControls()

	// Facts, right-aligned: what is loaded, at what scale, under which seed.
	facts := fmt.Sprintf("%d nodes   %.0f m/px", len(a.Nodes), a.view.MetresPerPixel)
	w := imgui.CalcTextSize(facts).X + 190
	if avail := imgui.ContentRegionAvail().X; avail > w+40 {
		imgui.SameLineV(imgui.CursorPosX()+avail-w, 0)
		textDim(facts)
		imgui.SameLine()
		// The seed, always visible and always editable. "A run that cannot be
		// reproduced is not evidence" - so it is never in a dialogue.
		textDim("seed")
		imgui.SameLine()
		imgui.SetNextItemWidth(76)
		seed := int32(a.runSeed())
		if imgui.InputIntV("##seed", &seed, 0, 0, 0) {
			a.seed = uint64(seed)
			a.buildEngine()
			a.saveConfig()
		}
		if imgui.IsItemHovered() {
			imgui.SetTooltip("Same seed, same scenario, same result.")
		}
	}
}

// drawViewTabs is the standout control: which of the four activities you are
// doing. A tab strip, not menu items - the old switcher sat between File and
// Windows, which is a category error and why nobody could find it.
func (a *App) drawViewTabs() {
	for w := workspace(0); w < workspaceCount; w++ {
		if w > 0 {
			imgui.SameLineV(0, 2)
		}
		active := w == a.ws
		if active {
			imgui.PushStyleColorVec4(imgui.ColButton, colAccent)
			imgui.PushStyleColorVec4(imgui.ColButtonHovered, colAccent)
			imgui.PushStyleColorVec4(imgui.ColText, rgb(0x0B, 0x0E, 0x12))
		}
		if imgui.ButtonV(w.String(), imgui.NewVec2(74, 0)) {
			a.switchWorkspace(w)
		}
		if active {
			imgui.PopStyleColorV(3)
		}
		if imgui.IsItemHovered() {
			imgui.SetTooltip(fmt.Sprintf("%s  (ctrl+%d)\n%s", w.String(), w+1, w.purpose()))
		}
	}
}

// drawToolRail is the placement palette, on the map's left edge where the
// stage tools live in every other spatial tool. Vertical, icon-width, and
// beside the thing it acts on rather than in a distant row.
func (a *App) drawToolRail(origin imgui.Vec2, h float32) {
	// Two groups: what a click does to what is already there, then what a
	// click adds. Single letters were unreadable as a set - "R C O E" is a
	// password, not a palette - so the placement tools carry the map's own
	// node glyphs and a separator marks where adding begins.
	tools := []struct {
		t         Tool
		glyph     string
		sepBefore bool
	}{
		{t: ToolSelect, glyph: "\u2196"},
		{t: ToolMove, glyph: "\u2725"},
		{t: ToolPlaceRepeater, glyph: "\u25cf", sepBefore: true},
		{t: ToolPlaceCompanion, glyph: "\u25cb"},
		{t: ToolPlaceObserver, glyph: "\u25c9"},
		{t: ToolPlaceCustom, glyph: "\u2739"},
	}
	size := symbolButtonSize()
	imgui.SetCursorScreenPos(imgui.NewVec2(origin.X+6, origin.Y+8))
	if imgui.BeginChildStrV("##toolrail", imgui.NewVec2(size.X+16, float32(len(tools))*(size.Y+6)+18),
		imgui.ChildFlagsFrameStyle, imgui.WindowFlagsNoScrollbar) {
		for _, tl := range tools {
			if tl.sepBefore {
				imgui.Separator()
			}
			active := a.tool == tl.t
			if active {
				imgui.PushStyleColorVec4(imgui.ColButton, colAccent)
				imgui.PushStyleColorVec4(imgui.ColText, rgb(0x0B, 0x0E, 0x12))
			}
			if imgui.ButtonV(tl.glyph+"##tool"+tl.t.label(), size) {
				a.tool = tl.t
			}
			if active {
				imgui.PopStyleColorV(2)
			}
			if imgui.IsItemHovered() {
				imgui.SetTooltip(tl.t.tip())
			}
		}
	}
	imgui.EndChild()
	// Measured, not computed: everything else on the map keeps clear of where
	// the rail actually ended up. Predicting it from font metrics is how the
	// filter box ended up underneath it.
	a.railRight = imgui.ItemRectMax().X
}

// drawStatusBar is the fixed bottom instrument: what just happened, and what
// is happening in the background.
//
// Fixed height and always present, because status that appears and vanishes
// pushed every panel down the moment the application had something to say -
// an instrument that moves the thing you were reading.
func (a *App) drawStatusBar() {
	vp := imgui.MainViewport()
	h := imgui.FrameHeight() + 10
	imgui.SetNextWindowPos(imgui.NewVec2(vp.Pos().X, vp.Pos().Y+vp.Size().Y-h))
	imgui.SetNextWindowSize(imgui.NewVec2(vp.Size().X, h))
	imgui.SetNextWindowViewport(vp.ID())
	imgui.PushStyleVarVec2(imgui.StyleVarWindowPadding, imgui.NewVec2(10, 4))
	flags := imgui.WindowFlagsNoTitleBar | imgui.WindowFlagsNoResize |
		imgui.WindowFlagsNoMove | imgui.WindowFlagsNoScrollbar |
		imgui.WindowFlagsNoSavedSettings | imgui.WindowFlagsNoDocking |
		imgui.WindowFlagsNoBringToFrontOnFocus
	imgui.BeginV("##statusbar", nil, flags)
	imgui.PopStyleVar()

	switch {
	case a.status != "":
		textColoured(colWarn, a.status)
		imgui.SameLine()
		if imgui.SmallButton("dismiss") {
			a.status = ""
		}
	case a.companionAttached():
		textColoured(colWarn, "1x locked: a companion client is attached, so simulated "+
			"time must track wall time")
	default:
		textDim(a.ws.String() + " - " + a.ws.purpose())
	}

	// Background work, always at the same end of the same bar.
	if jobs := a.activeJobs(); len(jobs) > 0 {
		label := fmt.Sprintf("jobs: %d", len(jobs))
		w := imgui.CalcTextSize(label).X + 30
		imgui.SameLineV(imgui.ContentRegionAvail().X-w+imgui.CursorPosX(), 0)
		if imgui.SmallButton(label) {
			imgui.OpenPopupStr("##jobs")
		}
		a.drawJobsPopupBody()
	}
	imgui.End()
}

// drawLayerPicker chooses what is drawn under the nodes.
//
// Hillshade is first and is the default. Every imagery layer contacts a third
// party whose terms have not been checked against how this application uses
// them, so choosing one is a decision the operator makes, and the map says
// whose data it is drawing for as long as it draws it.
func (a *App) drawLayerPicker() {
	current := "hillshade"
	if a.composite.HasBase {
		current = a.composite.Base.Name
	}
	imgui.SetNextItemWidth(140)
	if imgui.BeginCombo("##layer", current) {
		if imgui.SelectableBool("hillshade") {
			a.composite.HasBase = false
			a.terrainDirty = true
		}
		for _, l := range basemap.Layers() {
			if l.Kind != basemap.Base {
				continue
			}
			if imgui.SelectableBool(l.Name) {
				a.composite.Base, a.composite.HasBase = l, true
				a.layerID = l.ID
				a.tiles.forget()
				a.terrainDirty = true
				a.saveConfig()
			}
		}
		imgui.EndCombo()
	}

	imgui.SameLine()
	labels := a.composite.HasLabels
	if imgui.Checkbox("labels", &labels) {
		a.composite.HasLabels = labels
		if labels {
			for _, l := range basemap.Layers() {
				if l.Kind == basemap.Overlay {
					a.composite.Labels = l
					break
				}
			}
		}
		a.terrainDirty = true
	}
}

// attribution is what must appear while a layer is shown. Not a courtesy: every
// source used here requires it, and a map that drops it is a licence breach
// rather than an untidy screen.
func (a *App) attribution() string {
	parts := []string{"Elevation: AWS terrarium / national sources"}
	if a.composite.HasBase {
		parts = append(parts, a.composite.Base.Attribution)
	}
	if a.composite.HasLabels {
		parts = append(parts, a.composite.Labels.Attribution)
	}
	return strings.Join(parts, "   |   ")
}

// drawBottomTabs is everything that is about the run rather than the map.
//
// Tabs rather than a stack, because a link profile and a flood timeline are
// answers to different questions and nobody needs both at once — and stacking
// them means neither gets the height to be readable.
// SetNodes replaces the scenario.
//
// The view is refitted and the engine rebuilt, because a new set of nodes is a
// new geometry: every path loss that involves a node changes when it moves, and
// an engine carrying its old link cache forward would answer with a network that
// no longer exists.
func (a *App) SetNodes(nodes []scenario.Node) {
	a.Nodes = nodes
	a.selected, a.linkTo = -1, -1
	a.view.MetresPerPixel = 0 // refit on the next frame, when the size is known
	a.terrainDirty = true
	a.buildEngine()
	a.selectFirstLink()
}

// SetBasemapStore gives the map its tile source.
func (a *App) SetBasemapStore(s *basemap.Store) { a.bmStore = s }

// SetLayer picks the basemap to open with.
func (a *App) SetLayer(id string) error {
	l, ok := basemap.ByID(id)
	if !ok {
		return fmt.Errorf("ui: no basemap layer %q", id)
	}
	a.composite.Base, a.composite.HasBase = l, true
	a.layerID = id
	a.tiles.forget()
	return nil
}

// SetFetchTiles allows the map to download while panning.
func (a *App) SetFetchTiles(v bool) { a.fetchTiles = v }

// configuredUIScale is the operator's answer: the flag, or the environment.
//
// The environment hints matter because the common failure is the platform
// lying: under XWayland on a scaled desktop the X11 side reports a content
// scale of 1.0, KWin does not upscale the window, and the workbench renders
// at half size — "really small and scaled weirdly", verbatim.
func (a *App) configuredUIScale() float64 {
	if a.UIScale > 0 {
		return a.UIScale
	}
	// What ctrl+/- last chose outlives the session: an operator who zoomed
	// the UI once has answered the question, and asking again every launch
	// is what "settings that do not stick" feels like.
	if a.cfg.uiScale > 0 {
		return a.cfg.uiScale
	}
	for _, env := range []string{"MESHCORESIM_SCALE", "GDK_SCALE", "QT_SCALE_FACTOR"} {
		if v, err := strconv.ParseFloat(os.Getenv(env), 64); err == nil && v > 0.5 && v <= 4 {
			return v
		}
	}
	return 1
}

// applyUIScale rescales the whole UI to an absolute factor, live.
//
// ScaleAllSizes multiplies whatever the sizes currently are, so the ratio to
// the factor already applied is what gets passed — the fonts take the
// absolute value directly. This imgui re-rasterises fonts at the new size, so
// scaled text is sharp rather than a stretched bitmap.
func (a *App) applyUIScale(scale float64) {
	if scale < 0.5 {
		scale = 0.5
	}
	if scale > 3 {
		scale = 3
	}
	if scale == a.uiScale {
		return
	}
	imgui.CurrentStyle().ScaleAllSizes(float32(scale / a.uiScale))
	imgui.CurrentStyle().SetFontScaleMain(float32(scale))
	a.uiScale = scale
}

// requestUIScale queues a rescale for the top of the next frame — changing
// the style while half a frame is already laid out tears that frame.
func (a *App) requestUIScale(scale float64) {
	a.pendingScale = scale
}

// pumpUIScale runs at the top of the frame, before anything is drawn.
func (a *App) pumpUIScale() {
	if a.pendingScale == 0 {
		return
	}
	target := a.pendingScale
	a.pendingScale = 0
	a.applyUIScale(target)
	a.cfg.uiScale = a.uiScale
	a.saveConfig()
	a.status = fmt.Sprintf("UI scale %.0f%%  (ctrl +/- to adjust, ctrl 0 for automatic)", a.uiScale*100)
}

// defaultRadio is the preset a new or imported node starts on, with the
// toolbar's frequency honoured if the operator has changed it.
func (a *App) defaultRadio() scenario.RadioConfig {
	r := scenario.DefaultRadio()
	if a.freqMHz > 0 {
		r.CentreHz = a.freqMHz * 1e6
	}
	return r
}
