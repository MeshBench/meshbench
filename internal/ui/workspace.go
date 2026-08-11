package ui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AllenDang/cimgui-go/imgui"
)

// A workspace is a saved arrangement of panels for one kind of work.
//
// The UX spec named them from the start: an engineer debugging a collision
// wants the waterfall large, one planning a network wants the map large, and
// one fixed layout serves neither. Four presets, each rebuildable at any time,
// each remembering how the operator rearranged it.
type workspace int

const (
	wsPlan workspace = iota
	wsRun
	wsDebug
	wsVerify
	wsBench
	workspaceCount
)

func (w workspace) String() string {
	switch w {
	case wsRun:
		return "Run"
	case wsDebug:
		return "Debug"
	case wsVerify:
		return "Verify"
	case wsBench:
		return "Bench"
	default:
		return "Plan"
	}
}

// what each view is for, in the operator's terms - shown on hover, because
// a four-tab strip should not need a manual.
func (w workspace) purpose() string {
	switch w {
	case wsRun:
		return "exercise it and watch: play, schedule traffic, consoles, live feed"
	case wsDebug:
		return "ask why one thing happened: packets, waterfall, consoles, budgets"
	case wsVerify:
		return "check it is still true: baselines, A/B bisect, residuals against reality"
	case wsBench:
		return "compare configurations: sweep a parameter, repeat it, read what differed"
	default:
		return "build and site: import, place, drag, boundary, coverage"
	}
}

// panelSpec is one dockable panel: a name and what fills it.
type panelSpec struct {
	name string
	draw func()
	// open is per panel and survives workspace switches: closing the waterfall
	// is a statement about the waterfall, not about the Debug workspace.
	open bool
	// docked and ownWindow are recorded each frame while the panel draws —
	// the only place imgui can be asked safely — so the control socket can
	// report them without touching window internals off-frame, which is a
	// segfault wearing an API.
	docked    bool
	ownWindow bool
	// lastDock is the dock node the panel last sat in, so "dock" returns it
	// there rather than dumping it over the map in the central node.
	lastDock imgui.ID
}

// panelRegistry is every panel the workbench has, built once.
//
// The registry is what dissolves the old fixed layout: the bottom tab bar and
// the pinned sidebar were both just hard-coded lists of these, and a menu, a
// workspace preset and a docking layout can all be generated from one list
// where three hand-maintained ones would drift.
func (a *App) panelRegistry() []*panelSpec {
	if a.panelList != nil {
		return a.panelList
	}
	a.panelList = []*panelSpec{
		{name: "Inspector", draw: a.drawSelected, open: true},
		{name: "Nodes", draw: a.drawNodesPanel, open: true},
		{name: "Link", draw: a.drawAnalysis, open: true},
		{name: "Budget", draw: a.drawLinkBudget, open: false},
		{name: "Waterfall", draw: a.drawWaterfall, open: false},
		{name: "Packet timeline", draw: a.drawTimeGraph, open: true},
		{name: "Events", draw: a.drawTimeline, open: true},
		{name: "Scoreboard", draw: a.drawScoreboard, open: false},
		{name: "Console", draw: a.drawConsole, open: false},
		{name: "Schedule", draw: a.drawScheduleBody, open: false},
		{name: "Compare", draw: a.drawCompareBody, open: false},
		{name: "Sweep", draw: a.drawSweep, open: false},
		{name: "Runs", draw: a.drawRuns, open: false},
		{name: "Experiment log", draw: a.drawExperimentLog, open: false},
		{name: "Matrix", draw: a.drawMatrix, open: false},
		{name: "Configuration", draw: a.drawBenchConfig, open: false},
		{name: "Timelines", draw: a.drawTimelines, open: false},
		{name: "Validate", draw: a.drawValidateBody, open: false},
		{name: "Energy", draw: a.drawEnergyBody, open: false},
		{name: "Live feed", draw: a.drawLiveFeedBody, open: false},
		{name: "Import", draw: a.drawImportBody, open: false},
		// Places you work, so they are panels: dockable, poppable, part of a
		// view, saved in a layout. They were floating windows, which meant no
		// view could contain them and no layout could remember them.
		{name: "Boundary", draw: a.drawBoundaryBody, open: false},
		{name: "Planning", draw: a.drawPlanningBody, open: false},
		{name: "Fleet", draw: a.drawFleetBody, open: false},
	}
	return a.panelList
}

func (a *App) panelByName(name string) *panelSpec {
	for _, p := range a.panelRegistry() {
		if p.name == name {
			return p
		}
	}
	return nil
}

// layoutDir holds one imgui ini per workspace, so each remembers how the
// operator rearranged it.
func layoutDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return "layouts"
	}
	return filepath.Join(base, "meshcoresim", "layouts")
}

func (a *App) layoutFile(w workspace) string {
	return filepath.Join(layoutDir(), fmt.Sprintf("workspace-%d.ini", w))
}

// switchWorkspace saves the current arrangement and brings in the next.
func (a *App) switchWorkspace(w workspace) {
	if w == a.ws && !a.wsForce {
		return
	}
	_ = os.MkdirAll(layoutDir(), 0o755)
	// Save the outgoing arrangement — except on first boot, where "current" is
	// a pristine context. Saving that wrote an empty layout over the check
	// below, so the preset never built and every panel spawned floating: the
	// exact chaos presets exist to prevent.
	if !a.wsForce {
		imgui.SaveIniSettingsToDisk(a.layoutFile(a.ws))
	}
	a.ws = w
	a.wsForce = false
	if _, err := os.Stat(a.layoutFile(w)); err == nil {
		imgui.LoadIniSettingsFromDisk(a.layoutFile(w))
		a.wsRebuild = false
	} else {
		// Never arranged: the preset builds it on the next frame.
		a.wsRebuild = true
	}
	a.openPanelsFor(w)
}

// openPanelsFor makes a workspace show its own panels and only its own.
//
// The first version only ever opened, on the theory that closing a panel was
// a statement about the panel. In practice every switch accumulated: by the
// fourth workspace the bottom dock was ten tabs deep, every preset looked
// identical, and "messy" was the correct word. A workspace now means what it
// shows - with one exception: a panel popped out to its own OS window was
// placed there deliberately, on some other monitor, and stays.
func (a *App) openPanelsFor(w workspace) {
	names := map[workspace][]string{
		// Building and siting: hands on the map, one node at a time, with
		// the terrain profile wide enough to read.
		wsPlan: {"Inspector", "Nodes", "Import", "Link", "Boundary", "Planning"},
		// Exercising and watching: you touch little and observe a lot.
		wsRun: {"Schedule", "Scoreboard", "Live feed", "Events", "Packet timeline", "Console"},
		// Why did that happen: a chain of evidence, every link in reach.
		wsDebug: {"Inspector", "Budget", "Link", "Waterfall", "Console", "Events"},
		// Is it still true: the falsifiability workflow, given a front door.
		wsVerify: {"Compare", "Validate", "Scoreboard", "Events"},
		// Comparing configurations: the sweep, what it is doing, and what came
		// out. No map - an experiment is read, not sited.
		wsBench: {"Sweep", "Runs", "Experiment log", "Matrix", "Timelines", "Configuration"},
	}
	want := map[string]bool{}
	for _, n := range names[w] {
		want[n] = true
	}
	for _, p := range a.panelRegistry() {
		if p.ownWindow {
			continue
		}
		p.open = want[p.name]
	}
}

// buildWorkspace lays out the preset with the dock builder.
//
// Runs once per switch-to-unarranged-workspace, then imgui's own ini carries
// every later adjustment. The map is always the central node: it is the thing
// the panels are about.
func (a *App) buildWorkspace(dockID imgui.ID) {
	vp := imgui.MainViewport()
	imgui.InternalDockBuilderRemoveNode(dockID)
	root := imgui.InternalDockBuilderAddNodeV(dockID, imgui.DockNodeFlags(imgui.DockNodeFlagsDockSpace))
	imgui.InternalDockBuilderSetNodeSize(root, vp.Size())

	var right, bottom, centre imgui.ID
	centre = root
	// A right column at 0.24 was a column of wrapped prose: panels there hold
	// forms and tables, and the map loses less by giving them room than they
	// lose by not having it.
	imgui.InternalDockBuilderSplitNode(centre, imgui.DirRight, 0.30, &right, &centre)
	imgui.InternalDockBuilderSplitNode(centre, imgui.DirDown, 0.34, &bottom, &centre)

	placed := map[string]bool{"Map": true}
	dock := func(node imgui.ID, names ...string) {
		for _, n := range names {
			imgui.InternalDockBuilderDockWindow(n, node)
			placed[n] = true
		}
	}
	if a.ws != wsBench {
		imgui.InternalDockBuilderDockWindow("Map", centre)
	}

	switch a.ws {
	case wsRun:
		dock(right, "Schedule", "Scoreboard", "Live feed")
		dock(bottom, "Events", "Packet timeline", "Console")
	case wsDebug:
		// The map cedes room: debugging is reading, and the cut-through, the
		// waterfall and the consoles all want width. A 43 km terrain profile
		// in a 350 px column is a smear, which is what the old preset drew.
		dock(right, "Inspector", "Budget")
		dock(bottom, "Link", "Waterfall", "Console", "Events")
		imgui.InternalDockBuilderSetNodeSize(bottom, imgui.NewVec2(vp.Size().X, vp.Size().Y*0.5))
	case wsVerify:
		dock(right, "Compare", "Validate")
		dock(bottom, "Scoreboard", "Events")
	case wsBench:
		// The one view with no map. An experiment is read, not sited: the
		// question is how these configurations differ, and a map of Scotland
		// answers none of it while taking the best two thirds of the window.
		// The sweep sits left where it is defined, what it is doing sits right,
		// and what came out fills the middle.
		imgui.InternalDockBuilderDockWindow("Matrix", centre)
		dock(right, "Runs", "Experiment log")
		dock(bottom, "Configuration")
		dock(bottom, "Timelines")
		var left imgui.ID
		imgui.InternalDockBuilderSplitNode(centre, imgui.DirLeft, 0.34, &left, &centre)
		dock(left, "Sweep")
		imgui.InternalDockBuilderDockWindow("Matrix", centre)
		placed["Matrix"] = true
	default: // Plan
		dock(right, "Inspector", "Nodes", "Import", "Boundary", "Planning")
		dock(bottom, "Link", "Packet timeline")
	}
	// Every panel gets a home, whether the preset named it or not. An unplaced
	// panel spawns floating at a default position — which in practice meant on
	// top of the toolbar, looking like a layout bug rather than a choice.
	for _, p := range a.panelRegistry() {
		if !placed[p.name] {
			imgui.InternalDockBuilderDockWindow(p.name, bottom)
		}
	}
	imgui.InternalDockBuilderFinish(dockID)
}

// dockspaceID is the main window's dock node — the thing a panel docks back
// into when it is put away.
func dockspaceID() imgui.ID { return imgui.IDStr("msim-dock") }

// popOut queues a panel to become its own OS window on the next frame.
func (a *App) popOut(name string) {
	if a.detach == nil {
		a.detach = map[string]bool{}
	}
	a.detach[name] = true
}

// dockBack queues a panel to return to the main window.
func (a *App) dockBack(name string) {
	if a.redock == nil {
		a.redock = map[string]bool{}
	}
	a.redock[name] = true
}

// applyDockIntent carries out a queued pop-out or dock-back before Begin.
//
// The bug this fixes, twice reported and twice wrongly declared fixed: a
// *docked* window ignores SetNextWindowPos completely — its dock node owns
// its geometry — so the old detach button moved nothing and the panel sat
// exactly where it was. Undocking is a separate act (dock ID 0) and has to
// happen first; only then does a position outside the main viewport mean
// anything, and only then does imgui give the window its own platform window.
// pinToMainWindow keeps a window inside the main OS window.
//
// Without it, dragging any floating panel past the main window's edge turns
// it into an OS window of its own - which conflates two different acts.
// Floating (a window loose inside the workbench) and popping out (a window
// on another monitor) are separate decisions, and only the second is
// something an operator asks for explicitly.
func pinToMainWindow() {
	imgui.SetNextWindowViewport(imgui.MainViewport().ID())
}

// topMostClass makes a popped-out window stay above the main one, and keeps it
// out of the main viewport once it is there.
//
// NoAutoMerge belongs here, on the windows that asked to leave, rather than on
// the io config where it used to be. Set globally it applies to every floating
// window imgui has - including menu popups, combos and tooltips - so opening the
// Window menu spawned a decorated OS window for the dropdown, which the compositor
// then showed full screen and flashing. A panel deliberately put on a second
// monitor still should not merge back or disappear behind the window it was
// popped out of; a menu never wanted either.
func topMostClass() *imgui.WindowClass {
	c := imgui.NewWindowClass()
	c.SetViewportFlagsOverrideSet(imgui.ViewportFlagsTopMost | imgui.ViewportFlagsNoAutoMerge)
	return c
}

// applyWindowMode places a window according to whether it is popped out.
// Called before Begin for every panel and every node window.
func (a *App) applyWindowMode(name string) {
	if a.popped[name] {
		imgui.SetNextWindowClass(topMostClass())
		return
	}
	pinToMainWindow()
}

func (a *App) applyDockIntent(name string) {
	if a.undock[name] {
		delete(a.undock, name)
		imgui.SetNextWindowDockIDV(0, imgui.CondAlways)
		vp := imgui.MainViewport()
		imgui.SetNextWindowPosV(
			imgui.NewVec2(vp.Pos().X+vp.Size().X*0.3, vp.Pos().Y+vp.Size().Y*0.25),
			imgui.CondAlways, imgui.NewVec2(0, 0))
		imgui.SetNextWindowSizeV(a.windowSize(72, 22), imgui.CondAlways)
	}
	if a.detach[name] {
		delete(a.detach, name)
		if a.popped == nil {
			a.popped = map[string]bool{}
		}
		a.popped[name] = true
		imgui.SetNextWindowDockIDV(0, imgui.CondAlways)
		vp := imgui.MainViewport()
		imgui.SetNextWindowPosV(
			imgui.NewVec2(vp.Pos().X+vp.Size().X/3, vp.Pos().Y+vp.Size().Y/4),
			imgui.CondAlways, imgui.NewVec2(0, 0))
		imgui.SetNextWindowSizeV(a.windowSize(84, 28), imgui.CondAlways)
	}
}

// applyRedocks puts every window that asked to come home back in the main
// window. It runs where the layout builder is allowed to run — before the
// dockspace is submitted and before any panel — because DockBuilderFinish
// asserts if it is called while windows are being submitted, which is how the
// first attempt at "dock" took the process down.
//
// The root of a split dockspace is a parent node, and a dock request against
// a parent is only resolved to a leaf by Finish; queuing without finishing is
// why the button appeared to do nothing at all.
func (a *App) applyRedocks() {
	if len(a.redock) == 0 {
		return
	}
	// A panel goes back to the node it popped out of, when that node still
	// exists. A node that emptied when its last tab left is gone, and docking
	// into a dead node is a silent no-op - a remembered node counts as alive
	// only while some other docked panel still reports living in it. (Asking
	// imgui directly is out: the node lookup wraps a null pointer in a
	// non-nil value, and dereferencing it took the process down once.)
	alive := map[imgui.ID]bool{}
	for _, p := range a.panelRegistry() {
		if p.open && p.docked && p.lastDock != 0 && !a.redock[p.name] {
			alive[p.lastDock] = true
		}
	}
	for name := range a.redock {
		// No longer an OS window of its own, so it goes back to being pinned
		// inside the main one.
		delete(a.popped, name)
		target := dockspaceID()
		if p := a.panelByName(name); p != nil && alive[p.lastDock] {
			target = p.lastDock
		}
		imgui.InternalDockBuilderDockWindow(name, target)
		delete(a.redock, name)
	}
	imgui.InternalDockBuilderFinish(dockspaceID())
}

// drawPanels submits every open panel as a dockable window.
func (a *App) drawPanels() {
	for _, p := range a.panelRegistry() {
		if !p.open || !a.panelEnabled(p.name) {
			continue
		}
		a.applyDockIntent(p.name)
		a.applyWindowMode(p.name)
		open := p.open
		if imgui.BeginV(p.name, &open, imgui.WindowFlagsMenuBar) {
			p.docked = imgui.IsWindowDocked()
			if p.docked {
				p.lastDock = imgui.WindowDockID()
			}
			p.ownWindow = !p.docked &&
				imgui.WindowViewport().ID() != imgui.MainViewport().ID()
			a.panelChrome(p.name)
			p.draw()
		}
		imgui.End()
		p.open = open
	}
}

// panelChrome is the shared affordance strip at the top of every panel.
//
// One place, every panel, so "how do I get this onto my other monitor" has one
// answer instead of one per window.
// panelChrome is the panel's own menu bar.
//
// It was a button floated at the top-right of the body, which fought whatever
// the panel drew first: at narrow widths it sat on top of the content, and it
// clipped to "pop ou". A menu bar is part of the window frame, so it cannot
// overlap anything, it is in the same place in every panel, and it has room
// for the other per-panel verbs.
func (a *App) panelChrome(name string) {
	// Read *before* the menu bar opens: inside BeginMenuBar the current
	// window is the bar, not the panel, so IsWindowDocked answers about the
	// wrong thing - which is why a docked panel offered to dock itself.
	docked := imgui.IsWindowDocked()
	if !imgui.BeginMenuBar() {
		return
	}
	if imgui.BeginMenu(name) {
		if a.noViewports {
			imgui.TextDisabled("single-window: native Wayland forbids pop-out")
		} else if a.popped[name] {
			if imgui.MenuItemBool("bring back into the main window") {
				a.dockBack(name)
			}
		} else {
			if imgui.MenuItemBool("pop out to its own window") {
				a.popOut(name)
			}
			if imgui.IsItemHovered() {
				imgui.SetTooltip("A separate OS window, kept above this one, for another\n" +
					"monitor. Dragging a panel loose only floats it in here.")
			}
			if docked {
				if imgui.MenuItemBool("float inside this window") {
					a.floatInside(name)
				}
			} else if imgui.MenuItemBool("dock back") {
				a.dockBack(name)
			}
		}
		imgui.Separator()
		if imgui.MenuItemBool("close") {
			if p := a.panelByName(name); p != nil {
				p.open = false
			}
		}
		imgui.EndMenu()
	}
	imgui.EndMenuBar()
}

// drawNodesPanel is the node list with its filter — the old sidebar list,
// now a panel like everything else.
func (a *App) drawNodesPanel() {
	imgui.SetNextItemWidth(-1)
	imgui.InputTextWithHint("##nodefilter", "filter by name or kind", &a.nodeFilter, 0, nil)
	if imgui.BeginChildStrV("##nodelist", imgui.NewVec2(0, 0), 0, 0) {
		a.drawNodeRows()
	}
	imgui.EndChild()
}

// panelEnabled gates panels behind their feature switches. A disabled feature
// disappears entirely - menu, panel and buttons - rather than sitting greyed
// out asking to be explained.
func (a *App) panelEnabled(name string) bool {
	if name == "Energy" {
		a.ensureConfig()
		return a.cfg.energyEnabled
	}
	return true
}

// showPanel opens a panel and brings it to the front - the one way anything
// asks for a panel, now that Fleet, Boundary and Planning are panels rather
// than windows with their own flags.
func (a *App) showPanel(name string) {
	if p := a.panelByName(name); p != nil {
		p.open = true
		imgui.SetWindowFocusStr(name)
	}
}

// floatInside undocks a panel without popping it out: loose in the main
// window, which is what dragging a tab out now does.
func (a *App) floatInside(name string) {
	if a.undock == nil {
		a.undock = map[string]bool{}
	}
	a.undock[name] = true
}
