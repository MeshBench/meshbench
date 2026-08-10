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
	a.drawViewsMenu()
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
					a.status = fmt.Sprintf("%d frames written to %s - open it in Wireshark with "+
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
	a.drawRepeatersMenu()
	a.drawPlanningMenu()
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
	// The honesty line lives in the chrome (CLAUDE.md requires it said, not
	// findable) - at the end of the menu bar rather than on a row of its own,
	// which bought the map a full row back.
	imgui.SameLineV(0, 24)
	imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
	imgui.TextDisabled("results are a best case: no multipath, bare earth, ideal demodulator")
	imgui.PopStyleColor()
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
		if imgui.CurrentIO().KeyCtrl() && imgui.IsKeyPressedBool(imgui.KeyQ) {
			a.backend.SetShouldClose(true)
		}
		// Ctrl +/- zooms the whole UI, like a browser; ctrl 0 goes back to
		// automatic. Multiplicative steps, so each press feels the same size
		// at every scale.
		if imgui.CurrentIO().KeyCtrl() {
			if imgui.IsKeyPressedBool(imgui.KeyEqual) || imgui.IsKeyPressedBool(imgui.KeyKeypadAdd) {
				a.requestUIScale(a.uiScale * 1.1)
			}
			if imgui.IsKeyPressedBool(imgui.KeyMinus) || imgui.IsKeyPressedBool(imgui.KeyKeypadSubtract) {
				a.requestUIScale(a.uiScale / 1.1)
			}
			if imgui.IsKeyPressedBool(imgui.Key0) {
				a.cfg.uiScale = 0
				a.saveConfig()
				a.requestUIScale(1)
				a.status = "UI scale automatic again - takes full effect on the next launch"
			}
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
		// Undock-then-place, queued from the button below. Placing alone did
		// nothing while the window was docked, which is why this never worked.
		a.applyDockIntent(name)
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
	if imgui.IsWindowDocked() {
		if imgui.SmallButton("pop out") {
			a.popOut(n.Name)
		}
		if imgui.IsItemHovered() {
			imgui.SetTooltip("Make this a real OS window you can put on another monitor.")
		}
	} else if imgui.SmallButton("dock") {
		a.dockBack(n.Name)
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
			// One command, not four. MeshCore has `set radio <freq> <bw> <sf>
			// <cr>` and `set freq`; there is no `set bw`, `set sf` or `set cr`
			// — those answered "unknown config: bw 62.5" and the workbench
			// carried on as though the radio had changed, so the model and the
			// firmware silently disagreed about the modem from then on.
			//
			// `set radio` persists but needs a reboot, so `tempradio` applies
			// the same parameters to the running node immediately. Both, in
			// that order: the run behaves correctly now and the node comes back
			// configured.
			for _, cmd := range []string{
				fmt.Sprintf("set radio %.3f %g %d %d", p.FreqMHz, p.BwKHz, p.SF, p.CR),
				fmt.Sprintf("tempradio %.3f %g %d %d %d", p.FreqMHz, p.BwKHz, p.SF, p.CR,
					tempRadioMinutes),
			} {
				if err := en.Firmware.Bridge.Type([]byte(cmd + "\r\n")); err != nil {
					a.status = err.Error()
					return
				}
			}
			a.stepEngine(50)
			a.status = fmt.Sprintf("%s set to %s via its own CLI (applied now, persisted "+
				"for its next boot)", n.Name, p.Label)
			return
		}
	}
	a.recompute()
	a.status = fmt.Sprintf("%s radio set to %s", n.Name, p.Label)
}

// tempRadioMinutes is how long `tempradio` holds, in the firmware's own
// clock. Ten hours of simulated time: long enough that no ordinary run
// outlives it, and bounded because that is what the command is for.
const tempRadioMinutes = 600

// drawNodeStats is HopReach's per-repeater panel: what this node's airtime
// actually bought, and who it can really reach.
//
// Received against relayed, split by whether the relay reached anyone new. A
// repeater can be busy, legal, and reaching nobody who had not already heard
// the message — the number that decides whether a site is worth its airtime,
// and the one a duty-cycle figure hides completely.
func (a *App) drawNodeStats(name string) {
	if a.eng == nil {
		imgui.TextDisabled("no simulation yet - press play in the strip above")
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
		imgui.TextDisabled("no simulation yet - press play in the strip above")
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
	imgui.Separator()
	if imgui.MenuItemBoolV("Quit", "ctrl+q", false, true) {
		a.backend.SetShouldClose(true)
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

// drawRepeatersMenu is everything that is done *to* the repeaters, in one
// place — the fleet's commands, its provisioning, and its firmware.
//
// These were scattered across three windows reachable from two menus, which
// is why nobody could find them: the operator's question is "do something to
// my repeaters", not "open the fleet window".
func (a *App) drawRepeatersMenu() {
	if !imgui.BeginMenu("Repeaters") {
		return
	}
	running := a.eng != nil && a.eng.FirmwareCount() > 0
	if running {
		imgui.TextDisabled(fmt.Sprintf("%d on real firmware", a.eng.FirmwareCount()))
	} else {
		imgui.TextDisabled("no firmware running - start it from the strip above")
	}
	imgui.Separator()
	imgui.MenuItemBoolPtr("Fleet commands...", "", &a.winFleet)
	imgui.MenuItemBoolPtr("Firmware library...", "", &a.winFirmware)
	imgui.MenuItemBoolPtr("Provisioning (what they are told on boot)...", "", &a.winProvision)
	imgui.MenuItemBoolPtr("Nodes & settings...", "", &a.winNodesTable)
	imgui.Separator()

	// The commands people reach for, sent to every running repeater without a
	// detour through the fleet window. Each is a real CLI line and says so.
	if !running {
		imgui.TextDisabled("commands need firmware running")
		imgui.EndMenu()
		return
	}
	for _, q := range []struct{ label, cmd string }{
		{"advert now (all repeaters)", "advert"},
		{"read flood settings", "get flood.max"},
		{"read default scope", "region default"},
		{"list regions", "region"},
		{"save regions", "region save"},
	} {
		if imgui.MenuItemBool(q.label) {
			a.fleetSend(a.repeaterNames(), q.cmd)
			a.winFleet = true // the replies land there, so open it
		}
		if imgui.IsItemHovered() {
			imgui.SetTooltip(q.cmd)
		}
	}
	imgui.Separator()
	if imgui.MenuItemBool("set every repeater's region from the study area") {
		n := a.applyRegionsToFleet()
		a.status = fmt.Sprintf("region commands sent to %d repeaters", n)
		a.winFleet = true
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("region put <area> / region allowf <area> / region save\n" +
			"Uses each node's observed regions where inference found them,\n" +
			"and the study area's name otherwise.")
	}
	imgui.EndMenu()
}

// repeaterNames is every firmware-running repeater, for the fleet commands
// the Repeaters menu issues directly.
func (a *App) repeaterNames() []string {
	var out []string
	if a.eng == nil {
		return nil
	}
	for i := range a.Nodes {
		n := &a.Nodes[i]
		if !n.Kind.RunsFirmware() {
			continue
		}
		if en, ok := a.eng.NodeByName(n.Name); ok && en.Firmware != nil {
			out = append(out, n.Name)
		}
	}
	return out
}

// applyRegionsToFleet sends each running node its own region commands — the
// same lines Provisioning would issue at boot, to nodes already up.
func (a *App) applyRegionsToFleet() int {
	a.ensureConfig()
	sent := 0
	for i := range a.Nodes {
		cmds := a.regionCommands(i)
		if len(cmds) == 0 {
			continue
		}
		ok := true
		for _, cmd := range cmds {
			if err := a.typeAt(a.Nodes[i].Name, cmd); err != nil {
				ok = false
				break
			}
		}
		if ok {
			sent++
		}
	}
	if sent > 0 {
		a.stepEngine(50)
	}
	return sent
}

// drawPlanningMenu is where a network is designed rather than watched.
func (a *App) drawPlanningMenu() {
	if !imgui.BeginMenu("Planning") {
		return
	}
	imgui.MenuItemBoolPtr("Planning (bridge / cover an area)...", "", &a.winPlanning)
	imgui.MenuItemBoolPtr("Boundary (study area)...", "", &a.winBoundary)
	imgui.Separator()
	if imgui.MenuItemBool("coverage from the selected node") {
		if i, _ := a.Link(); i >= 0 {
			a.startCoverage(i)
		} else {
			a.status = "select a node first"
		}
	}
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
	if imgui.MenuItemBool("estimate terrain for this area") {
		if est, ok := a.terrainEstimate(); ok {
			a.status = fmt.Sprintf("%d tiles, %d cached, roughly %d MB to fetch",
				est.Tiles, est.Cached, est.BytesRough/1_000_000)
		} else {
			a.status = "this terrain source cannot estimate"
		}
	}
	if imgui.MenuItemBool("download terrain for this area") {
		a.fetchVisibleTerrain()
	}
	imgui.EndMenu()
}
