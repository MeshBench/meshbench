package ui

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/A13xB0/meshcoresim/internal/firmware"
	"os"
	"strings"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/antenna"
	"github.com/A13xB0/meshcoresim/internal/companion/proto"
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
	a.journal = append(a.journal, map[string]any{
		"at":     time.Now().UTC().Format(time.RFC3339),
		"method": "session.start",
		"note":   "workbench launched - any scenario from before this line is gone",
		"nodes":  len(a.Nodes),
	})
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
	if method == "session.journal" {
		return a.journal, nil
	}
	res, err := a.handleControlInner(method, params)
	a.recordCommand(method, params, err)
	return res, err
}

// recordCommand keeps the session's history, because a driven workbench has
// no other memory of what was done to it.
//
// An agent that reconnects - or one that restarted the process and does not
// know it - otherwise has to infer the state from what it can see, and the
// expensive mistake is exactly the one that is invisible: an inference run
// against a scenario somebody emptied. The journal answers "what has already
// happened here" in one call, and the launch entry makes a restart obvious.
func (a *App) recordCommand(method string, params json.RawMessage, err error) {
	if method == "ui.state" || method == "panels.list" || method == "status" ||
		method == "infer.result" {
		return // polls, not acts
	}
	e := map[string]any{
		"at":     time.Now().UTC().Format(time.RFC3339),
		"method": method,
		"nodes":  len(a.Nodes),
	}
	if len(params) > 0 && string(params) != "{}" {
		e["params"] = json.RawMessage(params)
	}
	if err != nil {
		e["error"] = err.Error()
	}
	a.journal = append(a.journal, e)
	// Bounded: a long session is a long session, not a leak.
	if len(a.journal) > 500 {
		a.journal = a.journal[len(a.journal)-500:]
	}
}

func (a *App) handleControlInner(method string, params json.RawMessage) (any, error) {
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
		// Asynchronous, across frames. This used to step the engine inside the
		// handler, which runs on the frame thread: nothing drew for the whole
		// run, so a flood could not be watched and long runs looked like a
		// hang. Poll sim.state, or just watch it.
		a.switchWorkspace(wsRun)
		a.runUntilMs = a.eng.NowMs() + p.ForMs
		a.playing = true
		return map[string]any{"running": true, "until_ms": a.runUntilMs,
			"now_ms": a.eng.NowMs()}, nil

	case "sim.state":
		if a.eng == nil {
			return map[string]any{"playing": false, "now_ms": 0}, nil
		}
		return map[string]any{"playing": a.playing, "now_ms": a.eng.NowMs(),
			"until_ms": a.runUntilMs, "events": a.eng.EventCount()}, nil

	case "sim.seed":
		// Repeats of the same seed are identical by design, so a study that
		// wants to know whether a difference is real rather than one draw has
		// to vary this.
		var p struct {
			Seed uint64 `json:"seed"`
		}
		_ = json.Unmarshal(params, &p)
		if p.Seed != 0 {
			a.seed = p.Seed
			a.buildEngine()
		}
		return map[string]any{"seed": a.runSeed()}, nil

	case "sim.reset":
		a.buildEngine()
		return map[string]any{"seed": a.runSeed()}, nil

	case "firmware.start":
		// Asynchronous: this handler runs on the frame thread, so starting a
		// few hundred real firmware processes here froze the window and the
		// socket that was driving it. Poll firmware.state.
		a.startFirmware()
		starting, done, total, _ := a.firmwareProgress()
		return map[string]any{"starting": starting, "done": done, "total": total}, nil

	case "app.quit":
		// Shut the workbench down from outside. A driven session otherwise
		// needs somebody at the machine to restart it after every rebuild,
		// which is most of what makes long automated runs impractical.
		a.quit = true
		return map[string]any{"quitting": true}, nil

	case "experiment.define":
		// The whole matrix in one call: arms, seeds, senders, timing. An agent
		// defines a sweep the same way a person does, and both then watch the
		// same run.
		var p struct {
			Arms []struct {
				Label            string `json:"label"`
				RepeaterVersion  string `json:"repeater_version"`
				CompanionVersion string `json:"companion_version"`
				PathHashMode     *int32 `json:"path_hash_mode"`
				LoopDetect       string `json:"loop_detect"`
				CAD              string `json:"cad"`
			} `json:"arms"`
			Seeds    []uint64 `json:"seeds"`
			Senders  []string `json:"senders"`
			Channel  string   `json:"channel"`
			Scope    string   `json:"scope"`
			SendAtMs uint32   `json:"send_at_ms"`
			RunForMs uint32   `json:"run_for_ms"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		e := a.ensureExperiment()
		if len(p.Arms) > 0 {
			e.Arms = nil
			for _, arm := range p.Arms {
				mode := int32(-1)
				if arm.PathHashMode != nil {
					mode = *arm.PathHashMode
				}
				e.Arms = append(e.Arms, expArm{
					Label: arm.Label, RepeaterVersion: arm.RepeaterVersion,
					CompanionVersion: arm.CompanionVersion, PathHashMode: mode,
					LoopDetect: arm.LoopDetect, CAD: arm.CAD,
				})
			}
		}
		if len(p.Seeds) > 0 {
			e.Seeds = p.Seeds
		}
		if len(p.Senders) > 0 {
			e.Senders = p.Senders
		}
		if p.Channel != "" {
			e.Channel = p.Channel
		}
		if p.Scope != "" {
			e.Scope = p.Scope
		}
		if p.SendAtMs > 0 {
			e.SendAtMs = p.SendAtMs
		}
		if p.RunForMs > 0 {
			e.RunForMs = p.RunForMs
		}
		return map[string]any{"arms": len(e.Arms), "seeds": len(e.Seeds),
			"runs": e.runsTotal(), "senders": len(e.Senders)}, nil

	case "experiment.vary":
		// The same gesture the operator makes: choose a parameter, type the
		// values, press add. Driven this way the form fills in on screen rather
		// than the state changing behind it, which matters when somebody is
		// watching the run happen.
		var p struct {
			Parameter string `json:"parameter"`
			Values    string `json:"values"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		a.switchWorkspace(wsBench)
		a.showPanel("Sweep")
		a.benchUI.param = p.Parameter
		a.benchUI.values = p.Values
		a.addArmsVarying(p.Parameter, p.Values)
		e := a.ensureExperiment()
		var labels []string
		for _, arm := range e.Arms {
			labels = append(labels, arm.Label)
		}
		return map[string]any{"arms": labels, "runs": e.runsTotal()}, nil

	case "experiment.seeds":
		var p struct {
			Seeds string `json:"seeds"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		a.switchWorkspace(wsBench)
		a.showPanel("Sweep")
		a.benchUI.seeds = p.Seeds
		e := a.ensureExperiment()
		if s := parseSeeds(p.Seeds); len(s) > 0 {
			e.Seeds = s
		}
		return map[string]any{"seeds": e.Seeds, "runs": e.runsTotal()}, nil

	case "experiment.base":
		// The experiment's constants, held across every arm.
		var p struct {
			LoopDetect   string `json:"loop_detect"`
			CAD          string `json:"cad"`
			PathHashMode *int32 `json:"path_hash_mode"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		e := a.ensureExperiment()
		if p.LoopDetect != "" {
			e.Base.LoopDetect = p.LoopDetect
		}
		if p.CAD != "" {
			e.Base.CAD = p.CAD
		}
		if p.PathHashMode != nil {
			e.Base.PathHashMode = *p.PathHashMode
		}
		a.switchWorkspace(wsBench)
		a.showPanel("Configuration")
		return map[string]any{"loop_detect": e.Base.LoopDetect, "cad": e.Base.CAD,
			"path_hash_mode": e.Base.PathHashMode}, nil

	case "experiment.senders":
		// Who originates the burst, and who only listens. Spread rather than
		// the first few in the list: a cluster of neighbours contends with
		// itself instead of with the mesh.
		var p struct {
			Mode     string   `json:"mode"`
			Count    int      `json:"count"`
			Nodes    []string `json:"nodes"`
			Observer string   `json:"observer"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		a.switchWorkspace(wsBench)
		a.showPanel("Sweep")
		e := a.ensureExperiment()
		switch {
		case len(p.Nodes) > 0:
			e.Senders = p.Nodes
		case p.Mode == "spread":
			n := p.Count
			if n <= 0 {
				n = 6
			}
			e.Senders = a.spreadSenders(n)
		}
		if p.Observer != "" {
			e.Observer = p.Observer
		}
		return map[string]any{"senders": e.Senders, "observer": e.Observer}, nil

	case "experiment.start":
		if err := a.startExperiment(); err != nil {
			return nil, err
		}
		return map[string]any{"running": true, "runs": a.exp.runsTotal()}, nil

	case "experiment.state":
		e := a.ensureExperiment()
		out := map[string]any{
			"running": e.running, "phase": phaseName(e.phase),
			"done": e.runsDone(), "total": e.runsTotal(), "status": e.status,
		}
		if n := len(e.log); n > 0 {
			out["log"] = e.log[max(0, n-12):]
		}
		return out, nil

	case "experiment.results":
		e := a.ensureExperiment()
		var rows []map[string]any
		for _, r := range e.results {
			rows = append(rows, map[string]any{
				"arm": r.Arm, "seed": r.Seed, "tx": r.TX, "rx": r.RX,
				"messages": r.Messages, "reach_pct": r.MeanReachPct,
				"reach_each": r.ReachPerMsg, "senders_of": r.SenderOf,
				"to_repeaters": r.RepHit, "repeater_chances": r.RepChances,
				"to_companions": r.CompHit, "companion_chances": r.CompChances,
				"repeats_per_msg": r.RepPerMsg, "comps_per_msg": r.CompPerMsg,
				"collisions": r.Collisions, "deaf": r.Deaf,
				"airtime_ms": r.AirtimeMs, "span_ms": r.SpanMs,
				"flag": r.Flag, "err": r.Err,
			})
		}
		var arms []map[string]any
		for _, s := range e.summarise() {
			arms = append(arms, map[string]any{
				"arm": s.Arm, "runs": s.Runs, "flagged": s.Flagged,
				"tx": s.TX, "rx": s.RX, "reach_pct": s.Reach,
				"messages": s.Messages, "repeater_pct": s.RepPct, "companion_pct": s.CompPct,
				"collisions": s.Coll, "deaf": s.Deaf, "airtime_ms": s.Airtime,
				"rx_spread": s.RXSpread,
			})
		}
		out := map[string]any{"runs": rows, "arms": arms}
		if w := e.notAResultYet(); w != "" {
			out["warning"] = w
		}
		if !e.running && len(e.results) > 0 {
			v := a.verdictFor(e)
			out["verdict"] = map[string]any{
				"difference": v.Difference, "headline": v.Headline,
				"investigation": v.Investigation, "detail": v.Detail,
			}
		}
		return out, nil

	case "experiment.compare":
		// First divergence between two runs: totals say something changed, this
		// says where.
		e := a.ensureExperiment()
		var p struct {
			ArmA, ArmB string `json:"arm_a"`
			Seed       uint64 `json:"seed"`
		}
		_ = json.Unmarshal(params, &p)
		if p.Seed == 0 && len(e.Seeds) > 0 {
			p.Seed = e.Seeds[0]
		}
		x := findResult(e.results, p.ArmA, p.Seed)
		y := findResult(e.results, p.ArmB, p.Seed)
		if x == nil || y == nil {
			return nil, fmt.Errorf("no run for %q and %q at seed %d", p.ArmA, p.ArmB, p.Seed)
		}
		return map[string]any{"comparison": firstDivergence(x.ledger, y.ledger)}, nil

	case "experiment.export":
		path, err := a.exportExperiment(a.ensureExperiment())
		if err != nil {
			return nil, err
		}
		return map[string]any{"path": path}, nil

	case "firmware.wipe":
		// Every node's persistent files: identity, prefs, channels, contacts.
		//
		// Needed between the arms of any comparison. The firmware writes prefs
		// and channels to its working directory and reads them back at boot, so
		// a second run inherits the first one's state - a version-B run that
		// started with version-A's contact database and channel table is not a
		// comparison, and nothing in the numbers says so.
		if err := firmware.WipeNodeStorage(); err != nil {
			return nil, fmt.Errorf("wiping node storage: %w", err)
		}
		return map[string]any{"wiped": true}, nil

	case "firmware.state":
		starting, done, total, errText := a.firmwareProgress()
		out := map[string]any{"starting": starting, "done": done, "total": total}
		if errText != "" {
			out["err"] = errText
		}
		if a.eng != nil {
			out["running"] = a.eng.FirmwareCount()
		}
		if !starting && total > 0 {
			// Configuration is the frame thread's job: it touches scenario
			// state the draw code reads.
			out["configured"] = a.applyStartupConfig()
		}
		return out, nil

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
		// Clamped, not reset: asking for more than the cap used to return 100,
		// so a caller asking for everything got less than one asking for
		// nothing. Use events.dump for the whole ledger.
		if p.Limit <= 0 {
			p.Limit = 100
		}
		if p.Limit > 500 {
			p.Limit = 500
		}
		return a.ctlEvents(p.Limit), nil

	case "events.dump":
		// The whole ledger, to a file. A run worth analysing is tens of
		// thousands of events, and a socket reply is the wrong shape for that.
		if a.eng == nil {
			return nil, fmt.Errorf("no simulation")
		}
		var pd struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(params, &pd)
		path := pd.Path
		if path == "" {
			return nil, fmt.Errorf("events.dump needs a path")
		}
		evs := a.events()
		out := make([]map[string]any, 0, len(evs))
		for _, ev := range evs {
			out = append(out, map[string]any{
				"at_ms": ev.AtMs, "kind": ev.Kind, "from": ev.From, "to": ev.To,
				"snr_db": ev.SNRdB, "detail": ev.Detail, "packet": ev.PacketID,
			})
		}
		b, err := json.Marshal(out)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return nil, err
		}
		return map[string]any{"events": len(out), "path": path}, nil

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
		row["popped_out"] = a.popped[p.name]
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
		a.ensureConfig()
		if a.cfg.realFirmware && a.eng != nil && a.eng.FirmwareCount() == 0 {
			a.attachFirmware()
		}
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
		a.showPanel("Planning")
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
		// Reveal what reports the work. A command that starts a fetch while
		// its panel is closed leaves the operator watching a window that
		// appears to be doing nothing - which is exactly what it looked like.
		a.switchWorkspace(wsPlan)
		a.showPanel("Import")
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
	case "boundary.set":
		a.switchWorkspace(wsPlan)
		a.showPanel("Boundary")
		a.bnd.query = arg("place")
		a.startBoundarySearch(a.bnd.query)
		return map[string]any{"searching": a.bnd.query}, true, nil
	case "boundary.accept":
		// Take the first result the search returned - the caller named the
		// place, and a socket cannot look at a list of candidates.
		if len(a.bnd.results) == 0 {
			return map[string]any{"chosen": len(a.bnd.chosen), "waiting": true}, true, nil
		}
		a.bnd.chosen = append(a.bnd.chosen, a.bnd.results[0])
		name := a.bnd.results[0].DisplayName
		a.bnd.results = nil
		return map[string]any{"added": name, "chosen": len(a.bnd.chosen)}, true, nil
	case "boundary.prune":
		before := len(a.Nodes)
		a.pruneOutside()
		return map[string]any{"before": before, "after": len(a.Nodes)}, true, nil
	case "infer.run":
		a.infer.ensureDefaults()
		if v, ok := num("hours"); ok && v > 0 {
			a.infer.lookbackH = int32(v)
		}
		if s := arg("regions"); s != "" {
			a.infer.extraRegions = s
		}
		a.switchWorkspace(wsPlan)
		a.showPanel("Import")
		a.startInference()
		return map[string]any{"reading": a.infer.lookbackH}, true, nil
	case "infer.result":
		if a.infer.running {
			return map[string]any{"running": true, "packets": a.infer.fetched.Load()}, true, nil
		}
		out := map[string]any{"running": false, "packets": a.infer.packets,
			"regions": a.infer.regions, "err": a.infer.err}
		nodes := map[string]any{}
		for n, v := range a.infer.result {
			nodes[n] = map[string]any{
				"regions": v.Regions, "default_scope": v.DefaultScope,
				"max_hops": v.MaxHops, "summary": v.Summary()}
		}
		out["nodes"] = nodes
		return out, true, nil
	case "infer.apply":
		// Through the panel's own counter, so a driven apply and a clicked one
		// leave the window saying the same thing. Driving it directly applied
		// the regions but left the panel reading "0 applied", which is
		// indistinguishable from having done nothing.
		a.infer.appliedN = a.applyInference()
		return map[string]any{"applied": a.infer.appliedN}, true, nil
	case "firmware.set":
		role, version := arg("role"), arg("version")
		if version == "" {
			return nil, true, fmt.Errorf("firmware.set needs a version")
		}
		n := 0
		for i := range a.Nodes {
			if !a.Nodes[i].Kind.RunsFirmware() {
				continue
			}
			if only := arg("node"); only != "" && !strings.EqualFold(only, a.Nodes[i].Name) {
				continue
			}
			if role != "" {
				a.Nodes[i].Firmware.Role = role
			}
			a.Nodes[i].Firmware.Version = version
			n++
		}
		a.rebuildForFirmware()
		return map[string]any{"nodes": n, "version": version}, true, nil
	case "sim.inject":
		i := a.nodeIndex(arg("node"))
		if i < 0 {
			return nil, true, fmt.Errorf("no node %q", arg("node"))
		}
		if a.eng == nil {
			a.buildEngine()
		}
		payload := arg("text")
		if payload == "" {
			payload = fmt.Sprintf("msim-%d", a.eng.NowMs())
		}
		a.eng.Inject(i, []byte(payload))
		return map[string]any{"from": a.Nodes[i].Name, "payload": payload}, true, nil
	case "companion.connect":
		if err := a.compConnect(arg("node")); err != nil {
			return nil, true, err
		}
		a.openNodeWindow(arg("node"))
		s := a.comps[arg("node")]
		s.mu.Lock()
		self := s.self
		s.mu.Unlock()
		out := map[string]any{"connected": arg("node")}
		if self != nil {
			out["name"] = self.Name
			out["freq_khz"] = self.FreqKHz
			out["sf"], out["cr"] = self.SF, self.CR
		}
		return out, true, nil
	case "companion.disconnect":
		a.compDisconnect(arg("node"))
		return map[string]any{"disconnected": arg("node")}, true, nil
	case "companion.state":
		s := a.comps[arg("node")]
		if s == nil {
			return nil, true, fmt.Errorf("not connected to %q", arg("node"))
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		chans := []map[string]any{}
		for _, c := range s.channels {
			chans = append(chans, map[string]any{"index": c.Index, "name": c.Name})
		}
		msgs := []map[string]any{}
		for _, m := range s.messages {
			msgs = append(msgs, map[string]any{
				"channel": m.ChannelIdx, "text": m.Text, "snr_db": m.SNRdB,
				"from": m.SenderName, "hops": m.PathLen, "mine": m.Mine,
				"at": m.At})
		}
		contacts := []map[string]any{}
		for _, c := range s.contacts {
			contacts = append(contacts, map[string]any{"name": c.Name, "hops": c.OutPathLen})
		}
		return map[string]any{"channels": chans, "messages": msgs,
			"contacts": contacts, "error": s.err}, true, nil
	case "companion.send":
		s := a.comps[arg("node")]
		if s == nil {
			return nil, true, fmt.Errorf("not connected to %q - companion.connect first", arg("node"))
		}
		cs := a.compUI[arg("node")]
		if cs == nil {
			cs = &compUIState{}
			if a.compUI == nil {
				a.compUI = map[string]*compUIState{}
			}
			a.compUI[arg("node")] = cs
		}
		// By name, because a caller says "#sco" and the firmware wants a slot.
		if want := arg("channel"); want != "" {
			found := false
			s.mu.Lock()
			for _, c := range s.channels {
				if strings.EqualFold(c.Name, want) {
					cs.channel, found = c.Index, true
				}
			}
			s.mu.Unlock()
			if !found {
				return nil, true, fmt.Errorf("no channel %q on this node", want)
			}
		}
		cs.scope = arg("scope")
		cs.draft = arg("text")
		a.compSendMessage(arg("node"), s, cs)
		return map[string]any{"sent": arg("text"), "channel": cs.channel}, true, nil
	case "project.save":
		// A driven pipeline otherwise has no way to survive the restart that
		// any rebuild forces, and an import plus a week of inference is the
		// better part of an hour to rebuild.
		name := arg("name")
		if name == "" {
			return nil, true, fmt.Errorf("project.save needs a name")
		}
		if err := a.saveProject(name); err != nil {
			return nil, true, err
		}
		return map[string]any{"saved": name, "nodes": len(a.Nodes)}, true, nil

	case "project.open":
		if err := a.openProject(arg("name")); err != nil {
			return nil, true, err
		}
		return map[string]any{"opened": arg("name"), "nodes": len(a.Nodes)}, true, nil

	case "project.list":
		var out []string
		for _, p := range listProjects() {
			out = append(out, p.name)
		}
		return map[string]any{"projects": out}, true, nil

	case "companion.add_channel":
		s := a.comps[arg("node")]
		if s == nil {
			return nil, true, fmt.Errorf("not connected to %q - companion.connect first", arg("node"))
		}
		idx, err := a.compAddChannel(s, arg("name"))
		if err != nil {
			return nil, true, err
		}
		return map[string]any{"node": arg("node"), "channel": arg("name"), "index": idx}, true, nil

	case "companion.raw":
		s := a.comps[arg("node")]
		if s == nil {
			return nil, true, fmt.Errorf("not connected to %q", arg("node"))
		}
		s.mu.Lock()
		var out []string
		for _, r := range s.rawMsgs {
			out = append(out, hex.EncodeToString(r))
		}
		s.mu.Unlock()
		return map[string]any{"frames": out}, true, nil

	case "companion.configure":
		// The companion equivalent of provisioning, so an agent can configure a
		// companion the way it configures a repeater. Without it, driving a run
		// from outside leaves companions unnamed and unscoped.
		node := arg("node")
		if a.comps[node] == nil {
			return nil, true, fmt.Errorf("not connected to %q - companion.connect first", node)
		}
		a.configureCompanion(node)
		return map[string]any{"node": node, "configured": true}, true, nil

	case "companion.advert":
		s := a.comps[arg("node")]
		if s == nil {
			return nil, true, fmt.Errorf("not connected to %q", arg("node"))
		}
		if err := a.compSend(s, proto.SendSelfAdvert(true)); err != nil {
			return nil, true, err
		}
		a.stepEngine(20)
		return map[string]any{"advertised": arg("node")}, true, nil
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
