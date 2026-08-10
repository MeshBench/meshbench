package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/antenna"
	"github.com/A13xB0/meshcoresim/internal/control"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// startControl opens the control socket.
//
// Best effort: a workbench that cannot listen is still a workbench, and
// refusing to start over a socket already in use would be the wrong trade.
func (a *App) startControl() {
	a.ensureConfig()
	if !a.cfg.controlEnabled {
		return
	}
	srv, err := control.Listen(a.handleControl)
	if err != nil {
		a.status = "control socket unavailable: " + err.Error()
		return
	}
	a.ctrl = srv
}

// stopControl closes the socket, so turning the switch off means the door is
// shut now rather than at the next launch.
func (a *App) stopControl() {
	if a.ctrl == nil {
		return
	}
	_ = a.ctrl.Close()
	a.ctrl = nil
}

// setControlEnabled is the Preferences switch. Opening and closing the socket
// live, because a setting that needs a restart to take effect is one nobody
// trusts they have actually turned off.
func (a *App) setControlEnabled(on bool) {
	a.cfg.controlEnabled = on
	a.saveConfig()
	if on {
		a.startControl()
		if a.ctrl != nil {
			a.status = "agent control enabled at " + a.ctrl.Path()
		}
		return
	}
	a.stopControl()
	a.status = "agent control disabled - the socket is closed and MCP tools will " +
		"report that no workbench is running"
}

// handleControl performs one command. Always on the frame thread, and always
// synchronously — a handler that blocked would stall the window it is meant to
// be driving, so nothing here waits on anything.
//
// The verbs are the actions an operator takes, not the functions the code
// happens to have — the ADR-0018 lesson about tool surfaces applies to this
// socket as much as to MCP: a dozen coarse commands, each answering something
// somebody actually wants to do.
func (a *App) handleControl(method string, params json.RawMessage) (any, error) {
	// The UI command surface first: chrome, navigation, windows, tools.
	if res, handled, err := a.handleUICommand(method, params); handled {
		return res, err
	}
	switch method {
	case "session.describe":
		return a.ctlDescribe(), nil

	case "nodes.list":
		return a.ctlNodes(), nil

	case "nodes.place":
		var p struct {
			Kind     string  `json:"kind"`
			Lat      float64 `json:"lat"`
			Lon      float64 `json:"lon"`
			Name     string  `json:"name"`
			HeightM  float64 `json:"height_m"`
			TxDBm    float64 `json:"tx_dbm"`
			Firmware string  `json:"firmware_role"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return a.ctlPlace(p.Kind, p.Name, p.Lat, p.Lon, p.HeightM, p.TxDBm, p.Firmware)

	case "nodes.delete":
		var p struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		i := a.nodeIndex(p.Name)
		if i < 0 {
			return nil, fmt.Errorf("no node named %q", p.Name)
		}
		a.DeleteNode(i)
		return map[string]any{"deleted": p.Name, "remaining": len(a.Nodes)}, nil

	case "nodes.move":
		var p struct {
			Name    string  `json:"name"`
			Lat     float64 `json:"lat"`
			Lon     float64 `json:"lon"`
			HeightM float64 `json:"height_m"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		i := a.nodeIndex(p.Name)
		if i < 0 {
			return nil, fmt.Errorf("no node named %q", p.Name)
		}
		if p.Lat != 0 || p.Lon != 0 {
			a.Nodes[i].Position = scenario.LatLon{Lat: p.Lat, Lon: p.Lon}
		}
		if p.HeightM > 0 {
			a.Nodes[i].HeightAGLm = p.HeightM
		}
		a.onGeometryChanged()
		a.startWarm()
		return a.ctlNodeAt(i), nil

	case "sim.run":
		var p struct {
			ForMs uint32 `json:"for_ms"`
		}
		_ = json.Unmarshal(params, &p)
		if p.ForMs == 0 {
			p.ForMs = 10_000
		}
		if a.eng == nil {
			a.buildEngine()
		}
		// Stepped rather than run in one call: the frame thread is blocked for
		// the duration, and a client asking for an hour of simulated time
		// should not freeze the window for a minute without saying so.
		steps := int(p.ForMs / a.eng.Config.StepMs)
		a.stepEngine(steps)
		return map[string]any{"now_ms": a.eng.NowMs(), "events": a.eng.EventCount()}, nil

	case "sim.reset":
		a.buildEngine()
		return map[string]any{"seed": a.runSeed()}, nil

	case "firmware.start":
		a.attachFirmware()
		if a.eng == nil {
			return nil, fmt.Errorf("no engine")
		}
		return map[string]any{"running": a.eng.FirmwareCount()}, nil

	case "console.type":
		var p struct {
			Node    string `json:"node"`
			Command string `json:"command"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		buf := a.consoleBufFor(p.Node)
		if buf == nil {
			return nil, fmt.Errorf("%s has no console; start firmware first", p.Node)
		}
		mark := buf.mark()
		if err := a.typeAt(p.Node, p.Command); err != nil {
			return nil, err
		}
		a.stepEngine(50)
		return map[string]any{"reply": buf.linesSince(mark)}, nil

	case "events.recent":
		var p struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal(params, &p)
		if p.Limit <= 0 || p.Limit > 500 {
			p.Limit = 100
		}
		return a.ctlEvents(p.Limit), nil

	case "scoreboard":
		if a.eng == nil {
			return nil, fmt.Errorf("no simulation")
		}
		return a.eng.Scoreboard(), nil

	case "radio.preset":
		var p struct {
			Node   string `json:"node"`
			Preset string `json:"preset"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		preset, ok := scenario.PresetByLabel(p.Preset)
		if !ok {
			return nil, fmt.Errorf("no preset %q", p.Preset)
		}
		applied := 0
		for i := range a.Nodes {
			if p.Node != "" && a.Nodes[i].Name != p.Node {
				continue
			}
			if !a.Nodes[i].Kind.Transmits() {
				continue
			}
			a.applyPreset(&a.Nodes[i], preset)
			applied++
		}
		return map[string]any{"applied_to": applied, "preset": preset.Label}, nil

	// Windows and layout. The workbench is meant to be drivable end to end —
	// "open the waterfall on my second monitor and save that as a view" is a
	// sentence, and every part of it is a verb here.
	case "panels.list":
		return a.ctlPanels(), nil
	case "panel.open":
		var p struct {
			Name string `json:"name"`
			Open *bool  `json:"open"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return a.ctlPanelOpen(p.Name, p.Open)
	case "panel.pop_out":
		var p struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		a.popOut(p.Name)
		if pl := a.panelByName(p.Name); pl != nil {
			pl.open = true
		}
		return map[string]any{"popped": p.Name}, nil
	case "panel.dock":
		var p struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		a.dockBack(p.Name)
		return map[string]any{"docked": p.Name}, nil
	case "workspace.set":
		var p struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		for w := workspace(0); w < workspaceCount; w++ {
			if strings.EqualFold(w.String(), p.Name) {
				a.switchWorkspace(w)
				return map[string]any{"workspace": w.String()}, nil
			}
		}
		return nil, fmt.Errorf("no workspace %q", p.Name)
	case "ui.scale":
		var p struct {
			Factor float64 `json:"factor"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		if p.Factor <= 0 {
			return map[string]any{"scale": a.uiScale}, nil
		}
		a.requestUIScale(p.Factor)
		return map[string]any{"requested": p.Factor}, nil
	case "view.save":
		var p struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		a.saveView(p.Name)
		return map[string]any{"saved": p.Name}, nil
	case "view.load":
		var p struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		a.loadView(p.Name)
		return map[string]any{"loaded": p.Name}, nil
	case "view.list":
		out := []map[string]any{}
		for _, v := range listViews() {
			out = append(out, map[string]any{"name": v.name, "saved": v.saved})
		}
		return out, nil
	case "view.delete":
		var p struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		a.deleteView(p.Name)
		return map[string]any{"deleted": p.Name}, nil
	case "assert.check":
		if a.eng == nil {
			return nil, fmt.Errorf("no simulation")
		}
		return a.eng.Check(a.sched.asserts), nil

	default:
		return nil, fmt.Errorf("unknown method %q", method)
	}
}

func (a *App) ctlDescribe() map[string]any {
	out := map[string]any{
		"nodes": len(a.Nodes),
		"seed":  a.runSeed(),
	}
	if a.eng != nil {
		out["now_ms"] = a.eng.NowMs()
		out["events"] = a.eng.EventCount()
		out["firmware_running"] = a.eng.FirmwareCount()
	}
	if r, _ := a.regionOrNil(); r != nil {
		s, n, w, e := r.Bounds()
		out["region"] = map[string]float64{"south": s, "north": n, "west": w, "east": e}
	}
	return out
}

func (a *App) ctlNodes() []map[string]any {
	out := make([]map[string]any, 0, len(a.Nodes))
	for i := range a.Nodes {
		out = append(out, a.ctlNodeAt(i))
	}
	return out
}

func (a *App) ctlNodeAt(i int) map[string]any {
	n := a.Nodes[i]
	m := map[string]any{
		"name": n.Name, "kind": string(n.Kind),
		"lat": n.Position.Lat, "lon": n.Position.Lon,
		"height_m": n.HeightAGLm, "tx_dbm": n.TxPowerDBm,
		"freq_mhz": n.Radio.CentreHz / 1e6, "sf": n.Radio.SpreadFactor,
	}
	if a.eng != nil {
		if en, ok := a.eng.NodeByName(n.Name); ok {
			m["sent"], m["heard"] = en.Sent, en.Heard
			m["firmware"] = en.Firmware != nil
		}
	}
	return m
}

func (a *App) ctlEvents(limit int) []map[string]any {
	if a.eng == nil {
		return nil
	}
	events := a.events()
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	out := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		out = append(out, map[string]any{
			"at_ms": ev.AtMs, "kind": ev.Kind, "from": ev.From, "to": ev.To,
			"snr_db": ev.SNRdB, "detail": ev.Detail, "packet": ev.PacketID,
		})
	}
	return out
}

// ctlPlace adds a node from outside.
func (a *App) ctlPlace(kind, name string, lat, lon, height, tx float64, role string) (any, error) {
	board, err := scenario.BoardByName(a.placeBoard)
	if err != nil {
		return nil, err
	}
	k := scenario.SimpleRepeater
	switch kind {
	case "companion":
		k = scenario.Companion
	case "observer", "sdr-observer":
		k = scenario.SDRObserver
	case "", "repeater", "simple-repeater":
	default:
		return nil, fmt.Errorf("unknown kind %q; have repeater, companion, observer", kind)
	}
	if height <= 0 {
		height = 10
	}
	if tx <= 0 && k.Transmits() {
		tx = board.MaxTxDBm
	}
	n := scenario.Node{
		Name: name, Kind: k,
		Position: scenario.LatLon{Lat: lat, Lon: lon}, HeightAGLm: height,
		TxPowerDBm: tx, NoiseFigureDB: board.NoiseFigureDB,
		Antenna: antenna.Mounted{
			Pattern:      antenna.Collinear{GainDBiPeak: board.AntennaDBi + 4},
			Polarisation: "vertical", FeedlineDB: board.FeedlineDB,
		},
		Radio:    scenario.RadioConfig{CentreHz: a.freqMHz * 1e6, BandwidthHz: 62500, SpreadFactor: 8, CodingRate: 4},
		Firmware: scenario.FirmwareRef{Role: role},
	}
	if n.Name == "" {
		n.Name = a.uniqueName(string(k))
	}
	if err := n.Validate(); err != nil {
		return nil, err
	}
	a.Nodes = append(a.Nodes, n)
	a.buildEngine()
	return a.ctlNodeAt(len(a.Nodes) - 1), nil
}

// ctlPanels is every panel, whether it is open, and — the question that
// matters for multi-monitor — whether it is currently its own OS window.
func (a *App) ctlPanels() map[string]any {
	panels := []map[string]any{}
	for _, p := range a.panelRegistry() {
		row := map[string]any{
			"name":    p.name,
			"open":    p.open,
			"enabled": a.panelEnabled(p.name),
		}
		// Docked, floating inside the main window, or a real OS window of its
		// own — the distinction the pop-out bug hid, made inspectable so a
		// script (or an agent) can assert it instead of eyeballing pixels.
		// Recorded at draw time; asking imgui for a window off-frame was a
		// segfault.
		row["docked"] = p.docked
		row["own_os_window"] = p.ownWindow
		panels = append(panels, row)
	}
	// Platform viewports beyond the main one are real OS windows. Counting
	// them is the only honest check that a pop-out worked: a window drawn at
	// an offset inside the main one looks identical in a screenshot.
	extra := 0
	if ctx := imgui.CurrentContext(); ctx != nil {
		if n := len(ctx.Viewports().Slice()); n > 1 {
			extra = n - 1
		}
	}
	return map[string]any{
		"panels":                 panels,
		"workspace":              a.ws.String(),
		"ui_scale":               a.uiScale,
		"os_windows_beyond_main": extra,
	}
}

func (a *App) ctlPanelOpen(name string, open *bool) (any, error) {
	p := a.panelByName(name)
	if p == nil {
		return nil, fmt.Errorf("no panel %q", name)
	}
	if open != nil {
		p.open = *open
	} else {
		p.open = true
	}
	return map[string]any{"name": name, "open": p.open}, nil
}

// The command surface: everything the chrome can do, doable from outside.
//
// The workbench is meant to be drivable end to end — "open the waterfall on
// my second monitor, switch to Debug, play for ten seconds and tell me what
// missed" is a sentence, and every verb in it has to exist here or the
// automation stops at the first button that has no name.
func (a *App) handleUICommand(method string, params json.RawMessage) (any, bool, error) {
	arg := func(key string) string {
		var m map[string]any
		if json.Unmarshal(params, &m) != nil {
			return ""
		}
		if v, ok := m[key].(string); ok {
			return v
		}
		return ""
	}
	num := func(key string) (float64, bool) {
		var m map[string]any
		if json.Unmarshal(params, &m) != nil {
			return 0, false
		}
		v, ok := m[key].(float64)
		return v, ok
	}

	switch method {
	case "ui.state":
		return a.ctlUIState(), true, nil

	// Transport - the run strip, as verbs.
	case "sim.play":
		a.playing = true
		return map[string]any{"playing": true}, true, nil
	case "sim.pause":
		a.playing = false
		return map[string]any{"playing": false}, true, nil
	case "sim.step":
		n := 20
		if v, ok := num("ticks"); ok && v > 0 {
			n = int(v)
		}
		a.stepEngine(n)
		return map[string]any{"now_ms": a.engNowMs()}, true, nil
	case "sim.speed":
		if v, ok := num("factor"); ok && v > 0 {
			a.speed = float32(v)
		}
		return map[string]any{"speed": a.speed}, true, nil

	// Navigation - where the map is looking, and what is selected.
	case "map.centre":
		lat, okLat := num("lat")
		lon, okLon := num("lon")
		if !okLat || !okLon {
			return nil, true, fmt.Errorf("map.centre needs lat and lon")
		}
		a.view.CentreLat, a.view.CentreLon = lat, lon
		if mpp, ok := num("metres_per_pixel"); ok && mpp > 0 {
			a.view.MetresPerPixel = mpp
		}
		a.terrainDirty = true
		return a.ctlView(), true, nil
	case "map.fit":
		a.view.FitTo(a.Nodes, a.view.Width, a.view.Height)
		a.terrainDirty = true
		return a.ctlView(), true, nil
	case "map.zoom":
		if f, ok := num("factor"); ok && f > 0 {
			a.view.MetresPerPixel /= f
			a.terrainDirty = true
		}
		return a.ctlView(), true, nil
	case "map.filter":
		a.nodeFilter = arg("text")
		return map[string]any{"filter": a.nodeFilter, "matches": len(a.matchingNodes())}, true, nil
	case "nodes.select":
		i := a.nodeIndex(arg("name"))
		if i < 0 {
			return nil, true, fmt.Errorf("no node %q", arg("name"))
		}
		var m map[string]any
		_ = json.Unmarshal(params, &m)
		add, _ := m["add_to_link"].(bool)
		a.SelectNode(i, add)
		from, to := a.Link()
		return map[string]any{"selected": from, "link_to": to}, true, nil

	// Tools and windows.
	case "tool.set":
		for _, t := range []Tool{ToolSelect, ToolMove, ToolPlaceRepeater,
			ToolPlaceCompanion, ToolPlaceObserver, ToolPlaceCustom} {
			if strings.EqualFold(t.label(), arg("tool")) {
				a.tool = t
				return map[string]any{"tool": t.label()}, true, nil
			}
		}
		return nil, true, fmt.Errorf("no tool %q", arg("tool"))
	case "window.open":
		return a.ctlWindow(arg("name"), true)
	case "window.close":
		return a.ctlWindow(arg("name"), false)
	case "node.window":
		name := arg("name")
		if a.nodeIndex(name) < 0 {
			return nil, true, fmt.Errorf("no node %q", name)
		}
		a.openNodeWindow(name)
		return map[string]any{"opened": name}, true, nil

	// Anything a menu can do to the fleet.
	case "fleet.send":
		targets := a.repeaterNames()
		if only := arg("node"); only != "" {
			targets = []string{only}
		}
		if len(targets) == 0 {
			return nil, true, fmt.Errorf("no firmware-running nodes")
		}
		a.fleetSend(targets, arg("command"))
		return map[string]any{"sent_to": len(targets), "command": arg("command")}, true, nil
	case "coverage.start":
		switch arg("mode") {
		case "best", "best-server", "":
			a.startNetworkCoverage(covBest)
		case "gaps":
			a.startNetworkCoverage(covGap)
		case "redundancy":
			a.startNetworkCoverage(covRedundancy)
		case "node":
			if i, _ := a.Link(); i >= 0 {
				a.startCoverage(i)
			} else {
				return nil, true, fmt.Errorf("select a node first")
			}
		}
		return map[string]any{"started": true}, true, nil
	case "coverage.clear":
		a.clearCoverage()
		return map[string]any{"cleared": true}, true, nil
	case "import.set_source":
		if s := arg("source"); s != "" {
			a.imp.source = s
		}
		if u := arg("url"); u != "" {
			a.imp.url = u
		}
		if t := arg("token"); t != "" {
			a.imp.token = t
		}
		return map[string]any{"source": a.imp.source, "url": a.imp.url}, true, nil
	case "import.fetch":
		a.startImportFetch()
		return map[string]any{"fetching": true}, true, nil
	case "import.commit":
		if a.imp.preview == nil {
			return nil, true, fmt.Errorf("no preview to commit - fetch first")
		}
		if s := arg("strategy"); s != "" {
			a.imp.strategy = scenario.MergeStrategy(s)
		}
		n := len(a.imp.preview.nodes)
		a.commitImport()
		return map[string]any{"committed": n, "nodes": len(a.Nodes)}, true, nil
	case "status":
		return map[string]any{"status": a.status}, true, nil
	}
	return nil, false, nil
}

// ctlUIState is one call that says everything about the shell: the view, the
// panels and where each of them is, the transport, the map, the selection.
// An agent driving the workbench should not need six round trips to find out
// what it is looking at.
func (a *App) ctlUIState() map[string]any {
	from, to := a.Link()
	sel := ""
	if from >= 0 && from < len(a.Nodes) {
		sel = a.Nodes[from].Name
	}
	linkTo := ""
	if to >= 0 && to < len(a.Nodes) {
		linkTo = a.Nodes[to].Name
	}
	views := []string{}
	for w := workspace(0); w < workspaceCount; w++ {
		views = append(views, w.String())
	}
	return map[string]any{
		"view":     a.ws.String(),
		"views":    views,
		"panels":   a.ctlPanels()["panels"],
		"playing":  a.playing,
		"speed":    a.speed,
		"now_ms":   a.engNowMs(),
		"tool":     a.tool.label(),
		"selected": sel,
		"link_to":  linkTo,
		"filter":   a.nodeFilter,
		"map":      a.ctlView(),
		"ui_scale": a.uiScale,
		"status":   a.status,
		"jobs":     len(a.activeJobs()),
		"firmware_running": func() int {
			if a.eng == nil {
				return 0
			}
			return a.eng.FirmwareCount()
		}(),
	}
}

func (a *App) ctlView() map[string]any {
	return map[string]any{
		"lat": a.view.CentreLat, "lon": a.view.CentreLon,
		"metres_per_pixel": a.view.MetresPerPixel,
	}
}

// ctlWindow drives the single-instance windows, which are flags rather than
// registry panels.
func (a *App) ctlWindow(name string, open bool) (any, bool, error) {
	switch strings.ToLower(name) {
	case "preferences":
		a.winPrefs = open
	case "provisioning":
		a.winProvision = open
	case "firmware library", "firmware":
		a.winFirmware = open
	case "nodes & settings", "nodes table":
		a.winNodesTable = open
	default:
		return nil, true, fmt.Errorf("no window %q", name)
	}
	return map[string]any{"window": name, "open": open}, true, nil
}
