package workbench

import (
	"strings"
	"testing"

	"gioui.org/f32"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// Do the buttons do anything?
//
// A screenshot proves a control was drawn. It cannot prove that pressing it
// reaches a verb, and this whole exercise started because a menu item had been
// dispatching a verb that does not exist.
//
// The text is set directly and only the buttons are clicked: where a field
// happens to sit is the layout's business and would make this a test of
// coordinates. Typing itself is covered separately.

type recorder struct {
	verbs  []string
	params []any
}

func (r *recorder) do(verb string, params any) {
	r.verbs = append(r.verbs, verb)
	r.params = append(r.params, params)
}

func (r *recorder) saw(verb string) bool {
	for _, v := range r.verbs {
		if v == verb {
			return true
		}
	}
	return false
}

// pressAlong clicks every few pixels across a row, so a button is found by
// being there rather than by a coordinate written down in advance.
func (h *panelHarness) pressAlong(y float32) {
	for x := float32(8); x < float32(h.sz.X); x += 12 {
		h.click(f32.Pt(x, y))
	}
}

func TestFleetControlsReachTheirVerbs(t *testing.T) {
	r := &recorder{}
	c := &fleetControls{do: r.do}
	h := newPanelHarness(c.Draw, &state.Snapshot{})
	h.frame()
	c.command.Editor.SetText("region put sco")
	c.regions.Editor.SetText("sco fif")
	h.frame()

	h.pressAlong(22)
	h.pressAlong(74)

	for _, want := range []string{"fleet.send", "nodes.regions", "nodes.allow_flood"} {
		if !r.saw(want) {
			t.Errorf("no button reached %s; got %v", want, r.verbs)
		}
	}
	for i, v := range r.verbs {
		if v != "fleet.send" {
			continue
		}
		m, _ := r.params[i].(map[string]any)
		if got, _ := m["command"].(string); got != "region put sco" {
			t.Errorf("fleet.send carried %q, not what the field holds", got)
		}
	}
	for i, v := range r.verbs {
		if v != "nodes.regions" {
			continue
		}
		m, _ := r.params[i].(map[string]any)
		rs, _ := m["regions"].([]any)
		if len(rs) != 2 {
			t.Errorf("nodes.regions carried %v, want two regions", rs)
		}
	}
}

func TestImportControlsReachAllFourSteps(t *testing.T) {
	r := &recorder{}
	c := &importControls{do: r.do}
	h := newPanelHarness(c.Draw, &state.Snapshot{})
	h.frame()
	c.url.Editor.SetText("https://example.test/")
	h.frame()

	h.pressAlong(22)

	for _, want := range []string{"import.fetch", "import.commit", "infer.run", "infer.apply"} {
		if !r.saw(want) {
			t.Errorf("no button reached %s; got %v", want, r.verbs)
		}
	}
}

func TestPlanningControlsAskAllThreeQuestions(t *testing.T) {
	r := &recorder{}
	c := &planningControls{do: r.do}
	h := newPanelHarness(c.Draw, &state.Snapshot{})
	h.frame()
	h.pressAlong(22)

	modes := map[string]bool{}
	for i, v := range r.verbs {
		if v != "coverage.start" {
			t.Errorf("planning reached %q", v)
			continue
		}
		m, _ := r.params[i].(map[string]any)
		mode, _ := m["mode"].(string)
		modes[mode] = true
	}
	for _, want := range []string{"best", "gaps", "redundancy", "node"} {
		if !modes[want] {
			t.Errorf("no button asked for %s coverage; got %v", want, modes)
		}
	}
}

func TestValidateControlsReachCalibration(t *testing.T) {
	r := &recorder{}
	c := &validateControls{do: r.do}
	h := newPanelHarness(c.Draw, &state.Snapshot{})
	h.frame()
	c.db.Editor.SetText("12.5")
	h.frame()
	h.pressAlong(22)

	for _, want := range []string{"validate.fetch", "validate.calibrate", "validate.uncalibrate"} {
		if !r.saw(want) {
			t.Errorf("no button reached %s; got %v", want, r.verbs)
		}
	}
	for i, v := range r.verbs {
		if v != "validate.calibrate" {
			continue
		}
		m, _ := r.params[i].(map[string]any)
		if got, _ := m["db"].(float64); got != 12.5 {
			t.Errorf("calibrate carried %v dB, not what the field holds", m["db"])
		}
	}
}

func TestBenchControlsReachTheirVerbs(t *testing.T) {
	r := &recorder{}
	c := &benchControls{do: r.do}
	h := newPanelHarness(c.Draw, &state.Snapshot{
		Nodes: []state.Node{{Name: "AngusOutlaw1", Kind: "companion", Selected: true}},
	})
	h.frame()
	c.msg.Editor.SetText("hello")
	h.frame()
	h.pressAlong(22)
	h.pressAlong(74)

	for _, want := range []string{"bench.serve", "bench.drop", "bench.stray",
		"companion.connect", "companion.send", "companion.advert"} {
		if !r.saw(want) {
			t.Errorf("no button reached %s; got %v", want, r.verbs)
		}
	}
	// With the node field blank it must act on the selection, not on "".
	for i, v := range r.verbs {
		if v != "companion.connect" {
			continue
		}
		m, _ := r.params[i].(map[string]any)
		if got, _ := m["node"].(string); got != "AngusOutlaw1" {
			t.Errorf("connect acted on %q, not the selected node", got)
		}
	}
}

// The arms come from the firmware library and the sender from the scenario, so
// both are chosen rather than typed. This drives the chooser the way the shell
// does: open the dropdown, and answer it.
func TestSweepArmsAndSenderArePicked(t *testing.T) {
	r := &recorder{}
	var asked []string
	c := &sweepControls{do: r.do}
	c.choose = func(_ string, opts []string, pick func(string)) {
		asked = opts
		if len(opts) > 0 {
			pick(opts[0])
		}
	}
	snap := &state.Snapshot{
		Builds: []state.Build{
			{Role: "simple_repeater", Version: "repeater-v1.17.0", Native: true},
			{Role: "simple_repeater", Version: "repeater-v1.17.1", Native: true},
			// Neither of these is an arm: one is a board image, the other a
			// different application entirely.
			{Role: "simple_repeater", Version: "v1.17.0", Board: "Generic_E22_sx1262"},
			{Role: "companion_radio", Version: "companion-v1.17.0", Native: true},
		},
		Nodes: []state.Node{
			{Name: "AngusOutlaw1", Kind: "companion"},
			{Name: "Lathenn Repeater", Kind: "repeater"},
		},
	}
	h := newPanelHarness(c.Draw, snap)
	h.frame()

	c.addArm.OnOpen()
	if len(asked) != 2 {
		t.Fatalf("offered %v as arms; want the two native repeater builds", asked)
	}
	c.sender.OnOpen()
	if len(asked) != 1 || asked[0] != "AngusOutlaw1" {
		t.Fatalf("offered %v as senders; only a companion can originate", asked)
	}
	// Picking an arm asks the session to cross it in rather than keeping a
	// copy here: the control socket defines arms too, and two copies disagree.
	if !r.saw("experiment.vary") {
		t.Fatalf("picking an arm reached %v, not experiment.vary", r.verbs)
	}
	if !r.saw("experiment.senders") {
		t.Fatalf("picking a sender reached %v, not experiment.senders", r.verbs)
	}
}

func TestSweepControlsDefineAndRun(t *testing.T) {
	r := &recorder{}
	c := &sweepControls{do: r.do}
	h := newPanelHarness(c.Draw, &state.Snapshot{})
	h.frame()
	// Arms and senders come from the session, so the harness supplies them the
	// way the store would.
	h.snap.ExperimentArms = []string{"1.16.0", "1.17.0"}
	h.snap.ExperimentSenders = []string{"AngusOutlaw1"}
	c.seeds.Editor.SetText("1 2 3")
	c.varyName = "repeater_version"
	c.varyVals.Editor.SetText("repeater-v1.16.0, repeater-v1.17.0")
	h.frame()
	// Bottom upwards. The buttons sit below the arm list, and each arm carries
	// a remove button: pressing downwards would take the arms off before
	// reaching the button that reads them.
	for y := float32(600); y >= 8; y -= 8 {
		h.pressAlong(y)
	}

	for _, want := range []string{"experiment.vary", "experiment.seeds",
		"experiment.start", "experiment.stop", "experiment.export"} {
		if !r.saw(want) {
			t.Errorf("no button reached %s; got %v", want, r.verbs)
		}
	}
	for i, v := range r.verbs {
		if v != "experiment.vary" {
			continue
		}
		m, _ := r.params[i].(map[string]any)
		vs, _ := m["values"].([]any)
		if len(vs) != 2 {
			t.Errorf("vary carried %v, want the two arms chosen", vs)
		}
	}
	for i, v := range r.verbs {
		if v != "experiment.seeds" {
			continue
		}
		m, _ := r.params[i].(map[string]any)
		ss, _ := m["seeds"].([]any)
		if len(ss) != 3 {
			t.Errorf("seeds carried %v, want three", ss)
		}
	}
}

func TestFeedControlsStartAndStop(t *testing.T) {
	r := &recorder{}
	c := &feedControls{do: r.do}
	h := newPanelHarness(c.Draw, &state.Snapshot{})
	h.frame()
	c.url.Editor.SetText("https://example.test/")
	h.frame()
	h.pressAlong(22)
	for _, want := range []string{"feed.pull", "feed.stop"} {
		if !r.saw(want) {
			t.Errorf("no button reached %s; got %v", want, r.verbs)
		}
	}
}

// The map toolbar: typing filters, and the tools select.
func TestMapToolbarFiltersAndPicksTools(t *testing.T) {
	mv := &comp.MapView{Zoom: 1000}
	m := &mapTools{mv: mv}
	h := newPanelHarness(
		func(t *theme.Theme, gtx layout.Context, _ *state.Snapshot) layout.Dimensions {
			return m.Draw(t, gtx)
		}, &state.Snapshot{})
	h.frame()

	// The filter applies as it is typed, with no button to press. The text is
	// set directly because where the box sits is the layout's business, and
	// typing itself is covered by the filter tests.
	m.filter.Editor.SetText("repeater")
	h.frame()
	if mv.Filter != "repeater" {
		t.Errorf("map filter is %q after typing", mv.Filter)
	}

	// A tool other than the default, found by pressing along the row.
	before := mv.Zoom
	h.pressAlong(22)
	if mv.Tool == "" || mv.Tool == "select" {
		t.Errorf("no tool was chosen; tool is %q", mv.Tool)
	}
	if mv.Zoom == before && !mv.FitNext {
		t.Errorf("neither zoom nor fit responded: zoom %v, fitNext %v", mv.Zoom, mv.FitNext)
	}
}

func TestInspectorIsTheLightEventsView(t *testing.T) {
	// The Inspector shows the selected node's own events, and clicking a row
	// opens the packet view directly - no intermediate pane.
	opened := uint64(0)
	p := &eventsPanel{compact: true, forNode: true,
		OnOpenPacket: func(id uint64) { opened = id }}
	h := newPanelHarness(p.Draw, &state.Snapshot{
		Nodes: []state.Node{{Name: "Bishop Hill", Selected: true}},
		Events: []state.Event{
			{AtMs: 1000, Kind: "tx", From: "Bishop Hill", PacketID: 7, Class: "sent"},
			{AtMs: 1200, Kind: "rx", From: "Leslie", To: "Bishop Hill", PacketID: 7, Class: "received"},
			{AtMs: 1300, Kind: "rx", From: "Leslie", To: "Markinch", PacketID: 8, Class: "received"},
		},
		EventTotal: 3,
	})
	h.frame()
	h.frame()
	key := eventKey(&state.Event{AtMs: 1000, Kind: "tx", From: "Bishop Hill", PacketID: 7, Class: "sent"})
	ck, ok := p.rows[key]
	if !ok {
		t.Fatalf("the selected node's own event was not drawn; rows: %d", len(p.rows))
	}
	for other := range p.rows {
		if other == eventKey(&state.Event{AtMs: 1300, Kind: "rx", From: "Leslie", To: "Markinch", PacketID: 8, Class: "received"}) {
			t.Error("an event that does not touch the selected node was drawn")
		}
	}
	ck.Click()
	h.frame()
	h.frame()
	if opened != 7 {
		t.Errorf("clicking the row opened packet %d, want 7", opened)
	}
}

// The client acts through verbs, not through command-line strings.
//
// Every button used to format a meshcore-cli line, which is why the panes
// could only ever show a terminal: the answer came back as text meant for
// one. Sending, scope, adverts and refresh are verbs now, and the CLI is a
// mode beside them rather than the thing underneath them.
func TestTheCompanionClientActsThroughVerbs(t *testing.T) {
	var verbs []string
	var lines []string
	c := &companionTab{
		node:  "AngusOutlaw1",
		OnCLI: func(_, line string) { lines = append(lines, line) },
		OnDo:  func(verb string, _ any) { verbs = append(verbs, verb) },
	}
	// The flat layout: the real one hides most controls behind the modes.
	h := newPanelHarness(
		func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return c.auditDraw(t, gtx, s)
		}, &state.Snapshot{})
	h.frame()
	c.msg.Editor.SetText("hello fife")
	c.scope.Editor.SetText("#sco")
	c.cmd.Editor.SetText("infos")
	h.frame()
	for y := float32(6); y < 340; y += 10 {
		h.pressAlong(y)
	}

	joined := strings.Join(verbs, " | ")
	for _, want := range []string{
		"companion.connect", "companion.send", "companion.scope",
		"companion.advert", "companion.refresh", "bench.serve", "bench.drop",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("no control reached %q; got: %s", want, joined)
		}
	}
	// And the command line still goes out as a command line.
	if !strings.Contains(strings.Join(lines, " | "), "infos") {
		t.Errorf("the CLI box did not send its line; got: %v", lines)
	}
}

// Serving hands the port to somebody else, so it has to let go of it first.
// Two holders of one claim is the thing the whole tab is arranged to prevent.
func TestServingDisconnectsFirst(t *testing.T) {
	var verbs []string
	c := &companionTab{
		node: "AngusOutlaw1",
		OnDo: func(verb string, _ any) { verbs = append(verbs, verb) },
	}
	c.build()
	snap := &state.Snapshot{Companions: []state.Companion{
		{Node: "AngusOutlaw1", Connected: true},
	}}
	h := newPanelHarness(
		func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return c.auditDraw(t, gtx, s)
		}, snap)
	h.frame()
	for y := float32(6); y < 340; y += 10 {
		h.pressAlong(y)
	}
	var disconnectAt, serveAt = -1, -1
	for i, v := range verbs {
		if v == "companion.disconnect" && disconnectAt < 0 {
			disconnectAt = i
		}
		if v == "bench.serve" && serveAt < 0 {
			serveAt = i
		}
	}
	if serveAt < 0 {
		t.Fatalf("Serve reached nothing: %v", verbs)
	}
	if disconnectAt < 0 || disconnectAt > serveAt {
		t.Errorf("served without releasing the claim first: %v", verbs)
	}
}

func TestProvisioningControlsReachTheirVerbs(t *testing.T) {
	r := &recorder{}
	c := &provisioningControls{do: r.do}
	h := newPanelHarness(c.Draw, &state.Snapshot{})
	h.frame()
	c.hops.Editor.SetText("3")
	c.stagger.Editor.SetText("250")
	c.extra.Editor.SetText("set tx 22")
	h.frame()
	for y := float32(14); y < 220; y += 14 {
		h.pressAlong(y)
	}

	if !r.saw("provisioning.set") {
		t.Fatalf("no button reached provisioning.set; got %v", r.verbs)
	}
	if !r.saw("provisioning.apply") {
		t.Errorf("nothing sends the settings to running nodes; got %v", r.verbs)
	}
	// The switches have to travel with it, defaults included.
	for i, v := range r.verbs {
		if v != "provisioning.set" {
			continue
		}
		m, _ := r.params[i].(map[string]any)
		if on, _ := m["set_name"].(bool); !on {
			t.Error("set_name did not travel with the settings")
		}
		if hops, _ := m["advert_hops"].(float64); hops != 3 {
			t.Errorf("advert_hops carried %v, not the typed 3", m["advert_hops"])
		}
		if x, _ := m["extra"].(string); x != "set tx 22" {
			t.Errorf("extra carried %q", x)
		}
		break
	}
}
