package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/firmware"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// drawMenuBar is the workbench's top-level structure.
//
// Menus are where features go to be found. The single-window layout kept
// everything visible at once, which stopped scaling the moment there was more
// than one of anything — a menu bar means a capability can exist without
// demanding permanent screen space.
func (a *App) drawMenuBar() {
	if !imgui.BeginMenuBar() {
		return
	}
	a.drawFileMenu()
	if imgui.BeginMenu("Simulation") {
		label := "run"
		if a.playing {
			label = "pause"
		}
		if imgui.MenuItemBoolV(label, "space", false, true) {
			a.playing = !a.playing
		}
		if imgui.MenuItemBoolV("step", ".", false, true) {
			a.stepEngine(20)
		}
		if imgui.MenuItemBool("reset") {
			a.buildEngine()
		}
		imgui.Separator()
		if a.eng != nil && a.eng.CapturePath() != "" {
			if imgui.MenuItemBool("stop capture") {
				path, frames, err := a.eng.StopCapture()
				if err != nil {
					a.status = err.Error()
				} else {
					a.status = fmt.Sprintf("%d frames written to %s — open it in Wireshark with "+
						"tools/dissector/meshcoresim.lua", frames, path)
				}
			}
		} else if imgui.MenuItemBool("capture to pcapng...") {
			path := filepath.Join(os.TempDir(), "meshcoresim.pcapng")
			if a.eng == nil {
				a.buildEngine()
			}
			if err := a.eng.StartCapture(path); err != nil {
				a.status = err.Error()
			} else {
				a.status = "capturing every receiver's view to " + path
			}
		}
		if imgui.IsItemHovered() {
			imgui.SetTooltip("Every node's view of every frame, in one file.\n" +
				"A real capture has one vantage point; this has all of them.")
		}
		if imgui.MenuItemBool("capture live to Wireshark...") {
			if a.eng == nil {
				a.buildEngine()
			}
			fifo := filepath.Join(os.TempDir(), "meshcoresim.fifo")
			if err := a.eng.StartCaptureFIFO(fifo); err != nil {
				a.status = err.Error()
			} else {
				a.status = "streaming to " + fifo + " - run: wireshark -k -i " + fifo +
					" -X lua_script:tools/dissector/meshcoresim.lua"
			}
		}
		if imgui.IsItemHovered() {
			imgui.SetTooltip("The same pcapng stream, into a named pipe Wireshark reads live.\n" +
				"Frames appear in Wireshark as the simulation produces them.")
		}
		imgui.Separator()
		if a.eng != nil && a.eng.EventLogPath() != "" {
			if imgui.MenuItemBool("stop event log") {
				path, lines, err := a.eng.StopEventLog()
				if err != nil {
					a.status = err.Error()
				} else {
					a.status = fmt.Sprintf("%d events written to %s", lines, path)
				}
			}
		} else if imgui.MenuItemBool("event log to NDJSON...") {
			if a.eng == nil {
				a.buildEngine()
			}
			path := filepath.Join(os.TempDir(), "meshcoresim-events.ndjson")
			if err := a.eng.StartEventLog(path); err != nil {
				a.status = err.Error()
			} else {
				a.status = "logging every event to " + path
			}
		}
		if imgui.IsItemHovered() {
			imgui.SetTooltip("One JSON object per event, as it happens - for jq, grep and diff.\n" +
				"Two runs of the same seed produce identical logs, so diffing them\n" +
				"is a regression test.")
		}
		imgui.Separator()
		if imgui.MenuItemBool("reset node memories") {
			// Rebuild first so no process is holding its files open, then wipe.
			a.buildEngine()
			if err := firmware.WipeNodeStorage(); err != nil {
				a.status = err.Error()
			} else {
				a.status = "node memories wiped - every node boots factory-fresh on the next " +
					"\"run real firmware\" (identities regenerate from the seed)"
			}
		}
		if imgui.IsItemHovered() {
			imgui.SetTooltip("Deletes every node's persisted identity, prefs and regions.\n" +
				"The cure for settings poisoned by an earlier bad run.")
		}
		imgui.Separator()
		if a.eng != nil && a.eng.FirmwareCount() == 0 {
			if imgui.MenuItemBool("run real firmware") {
				a.attachFirmware()
			}
		} else if a.eng != nil {
			imgui.TextDisabled(fmt.Sprintf("%d nodes on real firmware", a.eng.FirmwareCount()))
		}
		imgui.EndMenu()
	}
	if imgui.BeginMenu("Coverage") {
		if imgui.MenuItemBool("from the selected node") {
			if i, _ := a.Link(); i >= 0 {
				a.startCoverage(i)
			} else {
				a.status = "select a node first"
			}
		}
		imgui.Separator()
		if imgui.MenuItemBool("best server") {
			a.startNetworkCoverage(covBest)
		}
		if imgui.MenuItemBool("gaps - covered by nobody") {
			a.startNetworkCoverage(covGap)
		}
		if imgui.MenuItemBool("redundancy - what survives a failure") {
			a.startNetworkCoverage(covRedundancy)
		}
		imgui.Separator()
		imgui.TextDisabled("for a person with a handheld at 1.5 m")
		imgui.EndMenu()
	}
	// Workspaces, right in the bar rather than in a menu: switching is the
	// most common navigation there is, and Ctrl-1..4 are the fast path.
	a.drawWorkspaceSwitcher()
	if !imgui.CurrentIO().WantTextInput() && imgui.CurrentIO().KeyCtrl() {
		for w := workspace(0); w < workspaceCount; w++ {
			if imgui.IsKeyPressedBool(imgui.Key1 + imgui.Key(w)) {
				a.switchWorkspace(w)
			}
		}
	}
	if imgui.BeginMenu("Windows") {
		for _, p := range a.panelRegistry() {
			if a.panelEnabled(p.name) {
				imgui.MenuItemBoolPtr(p.name, "", &p.open)
			}
		}
		imgui.Separator()
		imgui.MenuItemBoolPtr("Preferences", "", &a.winPrefs)
		imgui.MenuItemBoolPtr("Provisioning", "", &a.winProvision)
		imgui.MenuItemBoolPtr("Nodes & settings", "", &a.winNodesTable)
		imgui.MenuItemBoolPtr("Fleet commands", "", &a.winFleet)
		imgui.MenuItemBoolPtr("Boundary", "", &a.winBoundary)
		imgui.MenuItemBoolPtr("Planning", "", &a.winPlanning)
		imgui.Separator()
		imgui.TextDisabled("node windows")
		// Every node can have its own window; the menu lists them so one can be
		// reopened without hunting for the node on a busy map.
		shown := 0
		for i := range a.Nodes {
			if shown >= 20 {
				imgui.TextDisabled("... use right-click on the map for the rest")
				break
			}
			name := a.Nodes[i].Name
			open := a.nodeWindows[name]
			if imgui.MenuItemBoolPtr(name, "", &open) {
				a.setNodeWindow(name, open)
			}
			shown++
		}
		imgui.EndMenu()
	}
	if imgui.BeginMenu("Help") {
		imgui.TextDisabled("Results are a best case: no multipath,")
		imgui.TextDisabled("bare-earth terrain, idealised demodulator.")
		imgui.TextDisabled("See docs/shortcomings.md.")
		imgui.EndMenu()
	}
	imgui.EndMenuBar()

	// The two shortcuts the menu advertises. Only when no text field owns the
	// keyboard, or typing a space into a node's console pauses the world.
	if !imgui.CurrentIO().WantTextInput() {
		if imgui.IsKeyPressedBool(imgui.KeySpace) {
			a.playing = !a.playing
		}
		if imgui.IsKeyPressedBool(imgui.KeyPeriod) {
			a.stepEngine(20)
		}
	}
}

// attachFirmware starts real MeshCore on every firmware-running node.
func (a *App) attachFirmware() {
	if a.eng == nil {
		a.buildEngine()
	}
	if err := a.eng.AttachNative(context.Background(), a.runSeed()); err != nil {
		// Reported, not fatal: AttachNative brings up everything it can, and
		// the nodes that did start are still worth configuring.
		a.status = err.Error()
	}
	// The nodes are running the firmware's built-in defaults until told
	// otherwise — including its default name, which every one of them shares.
	if n := a.applyStartupConfig(); n > 0 {
		a.status = fmt.Sprintf("%d nodes running MeshCore, %d configured",
			a.eng.FirmwareCount(), n)
		return
	}
	if a.status == "" {
		a.status = fmt.Sprintf("%d nodes running MeshCore", a.eng.FirmwareCount())
	}
}

func (a *App) openNodeWindow(name string) { a.setNodeWindow(name, true) }

func (a *App) setNodeWindow(name string, open bool) {
	if a.nodeWindows == nil {
		a.nodeWindows = map[string]bool{}
	}
	if open {
		a.nodeWindows[name] = true
	} else {
		delete(a.nodeWindows, name)
	}
}

// drawNodeWindows draws one floating window per opened node.
//
// This is HopReach's per-repeater detail, upgraded by not being a modal: any
// number can be open at once, so three repeaters can be watched side by side
// while the simulation runs — which a browser modal structurally cannot do.
func (a *App) drawNodeWindows() {
	for name := range a.nodeWindows {
		i := a.nodeIndex(name)
		if i < 0 {
			delete(a.nodeWindows, name)
			continue
		}
		open := true
		imgui.SetNextWindowSizeV(imgui.NewVec2(430, 360), imgui.CondFirstUseEver)
		// A window asked to detach is placed away from the main one before it
		// is drawn. imgui merges a floating window back into the main viewport
		// whenever the two overlap, so "outside" has to be somewhere the main
		// window is not — dragging alone cannot achieve that when it is
		// maximised, which is why this button exists.
		if a.detach[name] {
			delete(a.detach, name)
			vp := imgui.MainViewport()
			pos := vp.Pos()
			size := vp.Size()
			// Fully outside the main window — its right edge plus a margin.
			// The first attempt placed it 40 px *inside* that edge, which
			// merged straight back and made the button look like it did
			// nothing at all.
			imgui.SetNextWindowPosV(
				imgui.NewVec2(pos.X+size.X+40, pos.Y+80), imgui.CondAlways, imgui.NewVec2(0, 0))
		}
		if imgui.BeginV(name+"##nodewin", &open, 0) {
			a.drawNodeWindowBody(i)
		}
		imgui.End()
		if !open {
			delete(a.nodeWindows, name)
		}
	}
}

func (a *App) drawNodeWindowBody(i int) {
	n := &a.Nodes[i]

	// The header answers "what is this and what has it done" before any tab is
	// chosen.
	imgui.TextDisabled(kindLabel(n.Kind))
	if a.eng != nil {
		if en, ok := a.eng.NodeByName(n.Name); ok {
			imgui.SameLine()
			imgui.TextDisabled(fmt.Sprintf("|  sent %d  heard %d  airtime %.0f ms",
				en.Sent, en.Heard, en.AirtimeMs))
		}
	}

	imgui.SameLine()
	if imgui.SmallButton("detach") {
		if a.detach == nil {
			a.detach = map[string]bool{}
		}
		a.detach[n.Name] = true
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Move this window outside the main one, so it becomes a real OS\n" +
			"window you can put on another monitor. Dragging works too, but not\n" +
			"while the main window is maximised — there is nowhere outside it.")
	}

	if !imgui.BeginTabBar("##nodetabs") {
		return
	}
	if imgui.BeginTabItem("Console") {
		a.drawConsoleFor(n.Name)
		imgui.EndTabItem()
	}
	if imgui.BeginTabItem("Settings") {
		a.drawNodeSettings(i)
		imgui.EndTabItem()
	}
	if imgui.BeginTabItem("Stats") {
		a.drawNodeStats(n.Name)
		imgui.EndTabItem()
	}
	if imgui.BeginTabItem("Activity") {
		a.drawNodeActivity(n.Name)
		imgui.EndTabItem()
	}
	if n.Kind == scenario.Companion {
		if imgui.BeginTabItem("Connect") {
			a.drawCompanionTab(n.Name)
			imgui.EndTabItem()
		}
	}
	imgui.EndTabBar()
}

// drawNodeSettings is the node window's Settings tab: the same inspector
// component the Inspector panel draws, so a node's settings look identical
// everywhere they appear.
func (a *App) drawNodeSettings(i int) {
	a.drawNodeInspector(i)
}

// drawPresetCombo shows which community preset a node's radio matches, and
// applies one when picked.
func (a *App) drawPresetCombo(n *scenario.Node) {
	current := "custom"
	for _, p := range scenario.RadioPresets {
		if p.Matches(n.Radio) {
			current = p.Label
			break
		}
	}
	imgui.SetNextItemWidth(-70)
	if imgui.BeginCombo("preset", current) {
		for _, p := range scenario.RadioPresets {
			label := fmt.Sprintf("%s  (%.3f MHz, %g kHz, SF%d, CR4/%d)",
				p.Label, p.FreqMHz, p.BwKHz, p.SF, p.CR)
			if imgui.SelectableBool(label) {
				a.applyPreset(n, p)
			}
		}
		imgui.EndCombo()
	}
	imgui.TextDisabled(fmt.Sprintf("%.3f MHz  %g kHz  SF%d  CR4/%d",
		n.Radio.CentreHz/1e6, n.Radio.BandwidthHz/1000, n.Radio.SpreadFactor, n.Radio.CodingRate+4))
}

// applyPreset sets a node's radio, and tells its firmware if one is running.
//
// The scenario struct is the plan; the firmware's preference file is the truth.
// When a real build is attached the change goes through its own CLI, so it is
// validated, persisted and read back by the same code that would do it on a
// hilltop.
func (a *App) applyPreset(n *scenario.Node, p scenario.RadioPreset) {
	n.Radio = p.Config()
	if a.eng != nil {
		if en, ok := a.eng.NodeByName(n.Name); ok && en.Firmware != nil {
			for _, cmd := range []string{
				fmt.Sprintf("set freq %.3f", p.FreqMHz),
				fmt.Sprintf("set bw %g", p.BwKHz),
				fmt.Sprintf("set sf %d", p.SF),
				fmt.Sprintf("set cr %d", p.CR),
			} {
				if err := en.Firmware.Bridge.Type([]byte(cmd + "\r\n")); err != nil {
					a.status = err.Error()
					return
				}
			}
			a.stepEngine(50)
			a.status = fmt.Sprintf("%s set to %s via its own CLI", n.Name, p.Label)
			return
		}
	}
	a.recompute()
	a.status = fmt.Sprintf("%s radio set to %s", n.Name, p.Label)
}

// drawNodeStats is HopReach's per-repeater panel: what this node's airtime
// actually bought, and who it can really reach.
//
// Received against relayed, split by whether the relay reached anyone new. A
// repeater can be busy, legal, and reaching nobody who had not already heard
// the message — the number that decides whether a site is worth its airtime,
// and the one a duty-cycle figure hides completely.
func (a *App) drawNodeStats(name string) {
	if a.eng == nil {
		imgui.TextDisabled("no simulation yet - press run in the strip above")
		return
	}
	var s engine.Score
	var found bool
	for _, row := range a.eng.Scoreboard() {
		if row.Name == name {
			s, found = row, true
			break
		}
	}
	if !found {
		imgui.TextDisabled("this node is not in the run")
		return
	}

	stat := func(label, value string, col imgui.Vec4) {
		imgui.TextDisabled(label)
		imgui.SameLineV(140, -1)
		imgui.PushStyleColorVec4(imgui.ColText, col)
		imgui.Text(value)
		imgui.PopStyleColor()
	}
	plain := imgui.NewVec4(0.85, 0.88, 0.95, 1)
	stat("received", fmt.Sprint(s.Heard), plain)
	stat("relayed", fmt.Sprint(s.Sent), plain)
	stat("airtime", fmt.Sprintf("%.0f ms", s.AirtimeMs), plain)

	duty := plain
	if s.DutyCyclePct > 1 {
		duty = imgui.NewVec4(0.9, 0.4, 0.4, 1)
	}
	stat("duty cycle", fmt.Sprintf("%.2f%%", s.DutyCyclePct), duty)

	total := s.UniqueDelivery + s.RedundantRelay
	if total == 0 {
		stat("reach", "nothing relayed yet", imgui.NewVec4(0.6, 0.63, 0.7, 1))
	} else {
		ratio := float64(s.UniqueDelivery) / float64(total)
		col := imgui.NewVec4(0.45, 0.85, 0.5, 1)
		if ratio < 0.34 {
			col = imgui.NewVec4(0.9, 0.4, 0.4, 1)
		} else if ratio < 0.67 {
			col = imgui.NewVec4(0.95, 0.72, 0.25, 1)
		}
		stat("unique / redundant", fmt.Sprintf("%d / %d  (%.0f%% new)",
			s.UniqueDelivery, s.RedundantRelay, ratio*100), col)
		imgui.PushStyleColorVec4(imgui.ColPlotHistogram, col)
		imgui.ProgressBarV(float32(ratio), imgui.NewVec2(-1, 6), "")
		imgui.PopStyleColor()
	}

	imgui.SeparatorText("Neighbours")
	imgui.TextDisabled("who this node has actually exchanged packets with, and how well")
	a.drawNeighbours(name)
}

// drawNeighbours lists measured neighbours, both directions.
//
// From the ledger, not from a model: these are packets that were really
// exchanged during the run. Reachability is asymmetric, so heard-from and
// heard-by are separate columns — a neighbour a node can hear but cannot answer
// is the single most misleading thing a symmetric list would hide.
func (a *App) drawNeighbours(name string) {
	type link struct {
		inCount, outCount int
		inSNR, outSNR     float64
		seen              bool
	}
	links := map[string]*link{}
	get := func(peer string) *link {
		l, ok := links[peer]
		if !ok {
			l = &link{}
			links[peer] = l
		}
		return l
	}
	for _, ev := range a.events() {
		if ev.Kind != "rx" {
			continue
		}
		switch name {
		case ev.To:
			l := get(ev.From)
			l.inCount++
			l.inSNR, l.seen = ev.SNRdB, true
		case ev.From:
			l := get(ev.To)
			l.outCount++
			l.outSNR, l.seen = ev.SNRdB, true
		}
	}
	if len(links) == 0 {
		imgui.TextDisabled("none yet")
		return
	}
	if !imgui.BeginTableV("##neighbours", 3,
		imgui.TableFlagsBorders|imgui.TableFlagsRowBg|imgui.TableFlagsScrollY,
		imgui.NewVec2(0, 150), 0) {
		return
	}
	imgui.TableSetupColumnV("neighbour", imgui.TableColumnFlagsWidthStretch, 0, 0)
	imgui.TableSetupColumnV("heard from", imgui.TableColumnFlagsWidthFixed, 90, 0)
	imgui.TableSetupColumnV("heard by", imgui.TableColumnFlagsWidthFixed, 90, 0)
	imgui.TableHeadersRow()
	for peer, l := range links {
		imgui.TableNextRow()
		imgui.TableSetColumnIndex(0)
		imgui.Text(peer)
		imgui.TableSetColumnIndex(1)
		if l.inCount > 0 {
			imgui.Text(fmt.Sprintf("%+.1f dB", l.inSNR))
		} else {
			// One-way is the case worth seeing, so it is spelt out rather than
			// left as an empty cell.
			imgui.TextDisabled("never")
		}
		imgui.TableSetColumnIndex(2)
		if l.outCount > 0 {
			imgui.Text(fmt.Sprintf("%+.1f dB", l.outSNR))
		} else {
			imgui.TextDisabled("never")
		}
	}
	imgui.EndTable()
}

// drawNodeActivity is this node's slice of the event ledger, newest first.
func (a *App) drawNodeActivity(name string) {
	if a.eng == nil {
		imgui.TextDisabled("no simulation yet - press run in the strip above")
		return
	}
	events := a.eng.Events()
	if !imgui.BeginTableV("##nodeevents", 4,
		imgui.TableFlagsBorders|imgui.TableFlagsRowBg|imgui.TableFlagsScrollY,
		imgui.NewVec2(0, 0), 0) {
		return
	}
	imgui.TableSetupColumnV("t", imgui.TableColumnFlagsWidthFixed, 52, 0)
	imgui.TableSetupColumnV("with", imgui.TableColumnFlagsWidthFixed, 100, 0)
	imgui.TableSetupColumnV("SNR", imgui.TableColumnFlagsWidthFixed, 52, 0)
	imgui.TableSetupColumnV("what", imgui.TableColumnFlagsWidthStretch, 0, 0)
	imgui.TableHeadersRow()

	shown := 0
	for i := len(events) - 1; i >= 0 && shown < 200; i-- {
		ev := events[i]
		if ev.From != name && ev.To != name {
			continue
		}
		shown++
		other := ev.To
		if ev.To == name {
			other = ev.From
		}
		imgui.TableNextRow()
		imgui.TableSetColumnIndex(0)
		imgui.Text(fmt.Sprintf("%.2f", float64(ev.AtMs)/1000))
		imgui.TableSetColumnIndex(1)
		imgui.Text(other)
		imgui.TableSetColumnIndex(2)
		if ev.Kind != "tx" {
			imgui.Text(fmt.Sprintf("%+.1f", ev.SNRdB))
		}
		imgui.TableSetColumnIndex(3)
		imgui.PushStyleColorVec4(imgui.ColText, eventColour(ev))
		imgui.Text(ev.Detail)
		imgui.PopStyleColor()
	}
	imgui.EndTable()
	if shown == 0 {
		imgui.TextDisabled("nothing yet involving this node - run, or send from it")
	}
}

func (a *App) nodeIndex(name string) int {
	for i := range a.Nodes {
		if a.Nodes[i].Name == name {
			return i
		}
	}
	return -1
}

// drawFileMenu is projects and saved networks, where every desktop application
// keeps them. The Load panel that used to hold both is gone: opening work is a
// menu action, importing a network is the Import tool.
func (a *App) drawFileMenu() {
	if !imgui.BeginMenu("File") {
		return
	}
	if imgui.BeginMenu("Open project") {
		rows := listProjects()
		if len(rows) == 0 {
			imgui.TextDisabled("nothing saved yet")
		}
		for _, p := range rows {
			if imgui.MenuItemBool(fmt.Sprintf("%s  -  %d nodes, %s", p.name, p.nodes, age(p.saved))) {
				if err := a.openProject(p.name); err != nil {
					a.status = err.Error()
				} else {
					a.status = "opened " + p.name
				}
			}
		}
		imgui.EndMenu()
	}
	if imgui.BeginMenu("Save project as...") {
		imgui.TextDisabled("the network, its areas, the schedule and the seed - the whole study")
		imgui.SetNextItemWidth(220)
		imgui.InputTextWithHint("##projname", "name", &a.projName, 0, nil)
		imgui.SameLine()
		if imgui.Button("save##proj") {
			if err := a.saveProject(a.projName); err != nil {
				a.status = err.Error()
			} else {
				a.status = "project saved"
				a.projName = ""
				imgui.CloseCurrentPopup()
			}
		}
		imgui.EndMenu()
	}
	imgui.Separator()
	if imgui.BeginMenu("Saved networks") {
		a.drawSavedNetworksMenu()
		imgui.EndMenu()
	}
	if imgui.BeginMenu("Save network as...") {
		imgui.TextDisabled("the nodes alone; a project keeps the study around them")
		imgui.SetNextItemWidth(220)
		imgui.InputTextWithHint("##savename", defaultSaveName(len(a.Nodes)), &a.saveName, 0, nil)
		imgui.SameLine()
		if imgui.Button("save##net") {
			a.saveNetwork()
			imgui.CloseCurrentPopup()
		}
		imgui.EndMenu()
	}
	imgui.Separator()
	if imgui.MenuItemBool("Import a network...") {
		if p := a.panelByName("Import"); p != nil {
			p.open = true
			imgui.SetWindowFocusStr("Import")
		}
	}
	imgui.EndMenu()
}

// drawSavedNetworksMenu lists saved networks with the load/add/delete verbs on
// each row — what happens is decided where the click happens.
func (a *App) drawSavedNetworksMenu() {
	rows := a.savedNetworks()
	if len(rows) == 0 {
		imgui.TextDisabled("nothing saved yet - save once and reopening takes milliseconds")
		return
	}
	for i, n := range rows {
		if !imgui.BeginMenu(fmt.Sprintf("%s  -  %d nodes, %s##sv%d", n.name, n.nodes, age(n.saved), i)) {
			continue
		}
		if imgui.MenuItemBool("load, replacing the scenario") {
			a.loadSavedNet(n.name, true)
		}
		if imgui.MenuItemBool("add to the scenario") {
			a.loadSavedNet(n.name, false)
		}
		imgui.Separator()
		// Deleting takes two clicks on the same row: the first arms it, the
		// second does it. A single-click delete beside a load item is a
		// misclick with consequences.
		lbl := "delete"
		if a.confirmDelete == n.name {
			lbl = "delete - sure?"
		}
		if imgui.MenuItemBool(lbl) {
			if a.confirmDelete == n.name {
				_ = os.Remove(filepath.Join(scenarioDir(), n.name+".json"))
				a.confirmDelete = ""
				a.savedDirty = true
			} else {
				a.confirmDelete = n.name
			}
		}
		imgui.EndMenu()
	}
}
