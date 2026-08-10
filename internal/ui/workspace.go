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
	wsDebugRF
	wsFirmware
	workspaceCount
)

func (w workspace) String() string {
	switch w {
	case wsRun:
		return "Run"
	case wsDebugRF:
		return "Debug RF"
	case wsFirmware:
		return "Firmware"
	default:
		return "Plan"
	}
}

// panelSpec is one dockable panel: a name and what fills it.
type panelSpec struct {
	name string
	draw func()
	// open is per panel and survives workspace switches: closing the waterfall
	// is a statement about the waterfall, not about the Debug workspace.
	open bool
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
		{name: "Validate", draw: a.drawValidateBody, open: false},
		{name: "Energy", draw: a.drawEnergyBody, open: false},
		{name: "Live feed", draw: a.drawLiveFeedBody, open: false},
		{name: "Import", draw: a.drawImportBody, open: false},
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

// openPanelsFor opens the panels a workspace is about.
//
// Opens, never closes: a panel the operator had open stays open, because a
// workspace switch is a change of emphasis, not a decision about every window.
func (a *App) openPanelsFor(w workspace) {
	names := map[workspace][]string{
		wsPlan:     {"Inspector", "Nodes", "Link", "Import"},
		wsRun:      {"Events", "Packet timeline", "Schedule", "Scoreboard", "Compare"},
		wsDebugRF:  {"Waterfall", "Budget", "Link", "Packet timeline"},
		wsFirmware: {"Console", "Events"},
	}
	for _, n := range names[w] {
		if p := a.panelByName(n); p != nil {
			p.open = true
		}
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
	imgui.InternalDockBuilderSplitNode(centre, imgui.DirRight, 0.24, &right, &centre)
	imgui.InternalDockBuilderSplitNode(centre, imgui.DirDown, 0.32, &bottom, &centre)

	placed := map[string]bool{"Map": true}
	dock := func(node imgui.ID, names ...string) {
		for _, n := range names {
			imgui.InternalDockBuilderDockWindow(n, node)
			placed[n] = true
		}
	}
	imgui.InternalDockBuilderDockWindow("Map", centre)

	switch a.ws {
	case wsRun:
		dock(right, "Schedule", "Scoreboard", "Compare", "Inspector")
		dock(bottom, "Packet timeline", "Events")
	case wsDebugRF:
		// The waterfall large is the point of this workspace, so it takes the
		// bottom band and the map cedes the space.
		dock(right, "Budget", "Link", "Inspector")
		dock(bottom, "Waterfall", "Packet timeline", "Events")
	case wsFirmware:
		dock(right, "Inspector", "Nodes")
		dock(bottom, "Console", "Events")
	default: // Plan
		dock(right, "Inspector", "Nodes", "Import")
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

// drawWorkspaceSwitcher is the menu-bar control.
func (a *App) drawWorkspaceSwitcher() {
	for w := workspace(0); w < workspaceCount; w++ {
		sel := w == a.ws
		if sel {
			imgui.PushStyleColorVec4(imgui.ColButton, imgui.NewVec4(0.26, 0.42, 0.66, 1))
		}
		if imgui.SmallButton(w.String()) {
			a.switchWorkspace(w)
		}
		if sel {
			imgui.PopStyleColor()
		}
		imgui.SameLine()
	}
}

// drawPanels submits every open panel as a dockable window.
func (a *App) drawPanels() {
	for _, p := range a.panelRegistry() {
		if !p.open || !a.panelEnabled(p.name) {
			continue
		}
		// A detach queued from the chrome button: place the window fully
		// outside the main viewport, where — with viewports on — it becomes an
		// OS window of its own.
		if a.detach[p.name] {
			delete(a.detach, p.name)
			vp := imgui.MainViewport()
			imgui.SetNextWindowPosV(
				imgui.NewVec2(vp.Pos().X+vp.Size().X+40, vp.Pos().Y+80),
				imgui.CondAlways, imgui.NewVec2(0, 0))
			imgui.SetNextWindowSizeV(imgui.NewVec2(520, 420), imgui.CondAlways)
		}
		open := p.open
		if imgui.BeginV(p.name, &open, 0) {
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
func (a *App) panelChrome(name string) {
	avail := imgui.ContentRegionAvail()
	imgui.SameLineV(avail.X-24, 0)
	if imgui.SmallButton("^##detach-" + name) {
		if a.detach == nil {
			a.detach = map[string]bool{}
		}
		a.detach[name] = true
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Detach: push this panel outside the main window, where it becomes\n" +
			"an OS window you can move to another monitor. Dragging its tab out\n" +
			"works too. (Wayland cannot position windows; run without -wayland.)")
	}
	imgui.NewLine()
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
