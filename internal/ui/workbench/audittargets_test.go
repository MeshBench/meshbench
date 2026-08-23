// Every panel the audit walks, and what counts as a control on it.
//
// Kept apart from the walk itself so that adding a panel is an entry in a
// list rather than an edit to the machinery that reads it.
package workbench

import (
	"fmt"

	"gioui.org/layout"
	"github.com/MeshBench/meshbench/internal/app/resource"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// auditTargets is every panel that owns controls, wired to a recorder.
//
// Shared by the tests that press the buttons and the one that types into the
// boxes, so a panel added to one is audited by both.
func auditTargets(r *recorder) []target {
	// The file dialog is the platform's, not ours, so the audit cannot drive
	// it - but it can check the button reaches it. Without this the browse
	// buttons look like dead controls, which is the one thing this test
	// exists to catch, and it would have been silenced by exempting them.
	shell.Browse = func(title, _ string, _ shell.PathAsk) (string, error) {
		r.do("ui.browse", title)
		return "", nil
	}

	boards := &boardsPanel{do: r.do}
	fleet := &fleetControls{do: r.do}
	fleet.choose = func(title string, _ []string, _ func(string)) { r.do("ui.choose", title) }
	sched := &scheduleControls{do: r.do}
	imp := &importControls{do: r.do}
	// Adding an area searches and then offers what it found, so it goes
	// through its own callback rather than straight to a verb.
	imp.OnArea = func(q string) { r.do("boundary.set", q) }
	bound := &boundaryControls{do: r.do}
	valid := &validateControls{do: r.do}
	plan := &planningControls{do: r.do}
	bench := &benchControls{do: r.do}
	feed := &feedControls{do: r.do}
	// A dropdown's job is to open the chooser, so the audit has to answer one:
	// without it these three press and reach nothing, which is true of every
	// dropdown and not a fault in any of them.
	sweep := &sweepControls{do: r.do,
		choose: func(_ string, opts []string, pick func(string)) {
			if len(opts) > 0 {
				pick(opts[0])
			}
		},
		askText: func(_, _, _ string, got func(string)) { got("rxdelay.base") },
	}
	prov := &provisioningControls{do: r.do}

	resP := &resourcesPanel{}
	resP.Refresh = func() { r.do("resource.list", "") }
	resP.OnAction = func(verb string, _ map[string]any) { r.do(verb, "") }
	// A row is what carries the controls, so the audit needs one on disk and
	// one absent: Remove is deliberately disabled for the second, and a panel
	// with no rows would prove nothing at all.
	snapWithResources := auditSnapshot()
	// The state strings are resource.State's own: a fixture that invents its
	// own vocabulary audits a panel nobody will ever see.
	snapWithResources.Resources = []state.ResourceRow{
		{Kind: "softdevice", Name: "s140", Version: "6.1.1", Bytes: 155112,
			State: string(resource.OnDisk), Fetchable: true, Licensed: true,
			Why: "pairs with an application based at 0x26000"},
		{Kind: "softdevice", Name: "s140", Version: "7.3.0", Bytes: 160000,
			Estimated: true, State: string(resource.Available), Fetchable: true,
			Why: "not fetched"},
		// A cache that fills itself: Fetch is disabled here, which is the
		// state the other two cannot cover.
		{Kind: "terrain", Name: "terrain tiles", Bytes: 7_600_000_000,
			State: string(resource.OnDisk), Auto: true, Licensed: true,
			Why: "height data under every link budget"},
	}

	// Terms open on one row, so the licence box is drawn in the audit rather
	// than only when somebody presses the button on a running workbench.
	snapWithResources.Licence = state.LicenceText{
		Kind: "terrain", Name: "terrain tiles",
		Text: "Terrain heights are Copernicus DEM and NASA SRTM.",
	}

	nodes := &nodesPanel{}
	nodes.OnSelect = func(string) { r.do("nodes.select", nil) }
	nv := &nodeViewPanel{}
	// The build list is an overlay: its buttons do not exist until a firmware
	// cell has been clicked, and auditing them shut only proves they are shut.
	nv.pick.library = auditBuilds
	nv.pick.open("Abernethy Repeater")
	nv.OnAction = func(a string, n string) { r.do(a, n) }
	nv.OnFirmware = func(n string, b buildChoice) { r.do("node.set_firmware", b.Version) }
	nw := &nodeWindowPanel{node: "Abernethy Repeater"}
	snapWithConsole := auditSnapshot()
	snapWithConsole.ConsoleNode = "Abernethy Repeater"
	snapWithConsole.Console = []string{"   0.000  > advert"}
	nw.OnCommand = func(n, l string) { r.do("console.type", l) }
	nw.OnAction = func(a, n string) { r.do(a, n) }
	nw.OnServe = func(node, kind string) { r.do("bench.serve", kind) }
	nw.comp.OnCLI = func(n, l string) { r.do("console.cli", l) }
	nw.comp.OnDo = func(verb string, _ any) { r.do(verb, "") }
	nw.OnDo = nw.comp.OnDo
	cfgSets := &settings{}
	cfg := &configPanel{do: r.do, sets: cfgSets}
	// Choosing is the shell's overlay; what the audit can ask is whether
	// pressing the dropdown reaches the chooser at all.
	cfg.choose = func(title string, _ []string, _ func(string)) { r.do("ui.choose", title) }
	snapGPU := auditSnapshot()
	snapGPU.GPU = state.GPUState{Present: true, Enabled: true,
		Device: "Audit Graphics 3000", Backend: "vulkan"}
	cmpP := &comparePanel{OnSave: func() { r.do("run.save", nil) }}
	planP := &planPanel{OnRun: func() { r.do("plan.routes", nil) }}
	// The URL is asked for in the action bar, so this panel owns no controls.
	impP := &importPanel{}
	feedP := &feedPanel{OnPull: func() { r.do("feed.pull", nil) }}
	benchP := &benchPanel{
		OnAction: func(a, n string) { r.do(a, n) },
		OnSelect: func(n string) { r.do("nodes.select", n) },
	}
	// A companion selected, because the bench's actions are about one: with
	// a repeater selected it correctly offers nothing, and auditing that
	// only proves the guard works.
	snapWithCompanion := auditSnapshot()
	for i := range snapWithCompanion.Nodes {
		snapWithCompanion.Nodes[i].Selected = snapWithCompanion.Nodes[i].Kind == "companion"
	}

	targets := []target{
		{"Nodes running", nv, nv.Draw, nil,
			// Choosing a build closes the list, so it is reopened before each.
			func() { nv.pick.open("Abernethy Repeater") }, nil, buildSkips()},
		{"Node window", nw, nw.auditDraw, snapWithConsole,
			// The tab row is above everything, so a pointer moving down the
			// panel leaves the console before it reaches the send button.
			func() { nw.tab = 0 }, nil, nodeWindowSkips()},
		{"Node window: companion", &nw.comp, nw.comp.auditDraw, nil, nil, nil, nil},
		{"Compare", cmpP, cmpP.Draw, nil, nil, nil, nil},
		{"Planning (view)", planP, planP.Draw, nil, nil, nil, nil},
		{"Import (view)", impP, impP.Draw, nil, nil, nil, nil},
		{"Live feed (view)", feedP, feedP.Draw, nil, nil, nil, nil},
		{"Companion bench (view)", benchP, benchP.Draw, snapWithCompanion, nil, nil, nil},
		{"Boards", boards, boards.Draw, nil, nil, nil, nil},
		{"Fleet", fleet, fleet.Draw, nil, nil, nil, nil},
		{"Schedule", sched, sched.Draw, nil, nil, nil, nil},
		{"Import", imp, imp.Draw, nil, nil, nil, nil},
		{"Boundary", bound, bound.Draw, nil, nil, nil, nil},
		{"Validate", valid, valid.Draw, nil, nil, nil, nil},
		{"Planning", plan, plan.Draw, nil, nil, nil, nil},
		{"Companion bench", bench, bench.Draw, nil, nil, nil, nil},
		{"Live feed", feed, feed.Draw, nil, nil, nil, nil},
		{"Sweep", sweep, sweep.Draw, nil, nil, nil, map[string]string{
			// Choosing what to vary is not an action; "add arms" is the one
			// that crosses it, and it is audited.
			"varyDD.Btn": "picks the parameter; add arms is what applies it",
		}},
		{"Provisioning", prov, prov.Draw, nil, nil, nil, nil},
		{"Resources", resP, resP.Draw, snapWithResources, nil, nil, nil},
		// The flat layout, so every section's controls are on screen at once;
		// the sidebar's own switching is TestConfigurationSectionsSwitch.
		// Fired counts the settings generation too: the Interface controls
		// change the interface, not the simulation, and that is still doing
		// something.
		{"Configuration", cfg, cfg.auditDraw, snapGPU, nil,
			func() int {
				_, _, gen := cfgSets.get()
				return len(r.verbs)*1000 + int(gen)
			}, nil},
	}
	return targets
}

// auditSnapshot is a network with something in every list, so a panel that
// draws its controls only when it has data draws them.
func auditSnapshot() *state.Snapshot {
	return &state.Snapshot{
		Nodes: []state.Node{
			{Name: "Abernethy Repeater", Kind: "repeater", Lat: 56.3, Lon: -3.3, Selected: true},
			{Name: "Bishop Hill", Kind: "repeater", Lat: 56.2, Lon: -3.2},
			{Name: "AngusOutlaw1", Kind: "companion", Lat: 56.5, Lon: -3.0},
		},
		Stats: []state.NodeStat{
			{Name: "Abernethy Repeater", Backend: "native", Running: true, RSSBytes: 4 << 20},
			{Name: "Bishop Hill", Backend: "native", Running: true, RSSBytes: 4 << 20},
		},
	}
}

// nodeWindowSkips is what the node window is not expected to answer.
//
// It carries the build list's own skips, because the window grows the same
// control the Nodes running panel does and a list you have to scroll is not a
// list that is broken.
func nodeWindowSkips() map[string]string {
	skip := map[string]string{
		// The companion tab belongs to a companion, and this node is a
		// repeater. Its controls are audited on their own below.
		"comp.msg": "companion only", "comp.scope": "companion only",
		"comp.cmd": "companion only", "comp.sendMsg": "companion only",
		"comp.applyScope": "companion only", "comp.advertBtn": "companion only",
		"comp.refreshBtn": "companion only", "comp.runCmd": "companion only",
		"comp.connectBtn": "companion only", "comp.release": "companion only",
		"comp.serveBtn": "companion only", "comp.stopServeBtn": "companion only",
		"comp.dropBtn": "companion only", "comp.tcpChip": "companion only",
		"comp.newChan": "companion only", "comp.addChan": "companion only",
		"comp.ptyChip": "companion only",
		"comp.setName": "companion only", "comp.setFreq": "companion only",
		"comp.setBW": "companion only", "comp.setSF": "companion only",
		"comp.setCR": "companion only", "comp.setTx": "companion only",
		"comp.applyRadio": "companion only",
		// The node is running, so the head offers stop and not start.
		"start": "drawn only when the node is stopped",
		// Nothing is served in the audit snapshot, so the SDR pane
		// offers serve and not stop.
		"sdrStop": "drawn only while an observer is being served",
		// The build list, which this window draws over everything rather than
		// inside its flex - so the audit's flat layout, which has no overlay,
		// never lays it out at all.
		//
		// Both claims below are held by tests that use the real draw path
		// instead: TestTheSettingsTabOpensTheBuildList moves a pointer down
		// the Settings pane until something opens the list, and
		// TestTheNodeWindowChangesOneNodesFirmware then clicks over the
		// overlay itself and checks which verb it reached. That is stronger
		// evidence than the flat layout could give, not weaker: it is the
		// route somebody actually takes.
		"changeFw":    "opens the build list rather than reaching a verb",
		"pick.cancel": "drawn in the overlay, which the flat audit layout has not got",
		"pick.filter": "drawn in the overlay, which the flat audit layout has not got",
	}
	for i := range auditBuilds() {
		skip[fmt.Sprintf("pick.btns[%d]", i)] =
			"drawn in the overlay, which the flat audit layout has not got"
	}
	return skip
}

// auditBuilds is the library every audit sees: a fixed few, so the number of
// controls on the page is a property of the panel and not of the machine.
//
// Short enough that all of them fit in the overlay, which is what lets the
// audit require every one to be reachable rather than excusing a tail of them.
// Scrolling to a build below the fold is a real thing a real library does, and
// TestAnyBuildIsReachableByFiltering is where that is walked - through the
// filter, which is how somebody actually reaches the fortieth build.
//
// One board image among them on purpose: those carry a board and a role as
// well as a version, and label themselves differently for it.
func auditBuilds() []buildChoice {
	return []buildChoice{
		{Label: "v1.9.1", Version: "v1.9.1"},
		{Label: "v1.9.0", Version: "v1.9.0"},
		{Label: "v1.8.2", Version: "v1.8.2"},
		{Label: "wadamesh-local", Version: "wadamesh-local"},
		{Label: "Heltec_v3 - repeater v1.9.1", Version: "v1.9.1",
			Board: "Heltec_v3", Role: "repeater"},
	}
}

// buildSkips names what the sweep is not expected to land on in the build list.
func buildSkips() map[string]string {
	return map[string]string{
		// Cancel closes the list. That is the whole of its job, so it reaches
		// no verb by design.
		"pick.cancel": "closes the list rather than reaching a verb",
	}
}

type target struct {
	name string
	ctrl any
	draw func(*theme.Theme, layout.Context, *state.Snapshot) layout.Dimensions
	// snap overrides the shared one where a panel draws nothing without
	// something particular in it.
	snap *state.Snapshot
	// reset puts the panel back into the state a control needs, before
	// each press.
	reset func()
	// fired overrides how "it did something" is counted, for a panel that
	// changes something other than the simulation.
	fired func() int
	// skip names controls that are not expected to answer here, and why.
	// Every entry is a claim that has to be true, not a way of quieting
	// the test: a control listed for the wrong reason is a control nobody
	// is checking.
	skip map[string]string
}
