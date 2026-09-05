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
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/theme"
	"github.com/MeshBench/meshbench/internal/ui/uitest"
	"github.com/MeshBench/meshbench/internal/ui/workbench/boardview"
	"github.com/MeshBench/meshbench/internal/ui/workbench/nodeview"
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

	boards := &nodeview.BoardsPanel{Do: r.do}
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
	snapWithResources := uitest.Snapshot()
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
			Why:   "height data under every link budget",
			HowTo: "fills itself as the map is used"},
		// And a cache that is fetched from somewhere else entirely, which is
		// the only row that carries the button to that somewhere.
		{Kind: "buildings", Name: "building footprints",
			State: string(resource.Available), Licensed: true,
			Why:        "heights and materials that stand in the way of a signal",
			HowTo:      "fetched from Configuration > Environ",
			HowToPanel: "Configuration"},
	}

	// Terms open on one row, so the licence box is drawn in the audit rather
	// than only when somebody presses the button on a running workbench.
	snapWithResources.Licence = state.LicenceText{
		Kind: "terrain", Name: "terrain tiles",
		Text: "Terrain heights are Copernicus DEM and NASA SRTM.",
	}

	setP := &setupPanel{}
	setP.Refresh = func() { r.do("setup.check", "") }
	setP.OnAction = func(verb string, _ map[string]any) { r.do(verb, "") }
	// One row per state the panel can draw, because the button is what carries
	// the action and three of the five states deliberately have none. A blocked
	// row offering a fetch is the fault this fixture exists to catch.
	snapWithSetup := uitest.Snapshot()
	snapWithSetup.Setup = []state.SetupGroup{{
		Name: "This build", Note: "everything below is per machine",
		Rows: []state.SetupRow{{
			Name: "this build", State: string(state.SetupReady),
			What: "MeshBench dev", Where: "/usr/local/bin/meshbench",
			Do: "nothing is bundled beside the binary"}, {
			// The one row that offers a download of the application itself.
			// Drawn here in the state it matters in: a newer release exists,
			// its size is stated, and the button has not been pressed.
			// The one row that offers a download of the application itself,
			// drawn in the state it matters in: a newer release exists, its
			// size is stated, and nothing has been pressed.
			Name: "version", State: string(state.SetupUndecided),
			What: "which release this is, and whether a newer one exists",
			Cost: "44.0 MB to download",
			Do:   "0.2.0 is out; downloading replaces nothing.",
			Verb: "update.download"}},
	}, {
		Name: "Firmware", Note: "cached in ~/.cache/meshbench/firmware",
		Rows: []state.SetupRow{{
			Name: "native builds", State: string(state.SetupNeeded),
			What: "MeshCore compiled for this machine", Cost: "a few megabytes a role",
			Do: "open Firmware and download one", Verb: "panel.open",
			Params: map[string]any{"name": "Firmware"}}},
	}, {
		Name: "Terrain",
		Rows: []state.SetupRow{{
			Name: "terrain heights", State: string(state.SetupUndecided),
			What: "the ground every link budget is measured over",
			Cost: "several hundred megabytes for a country",
			Do:   "nothing has been downloaded yet", Verb: "terrain.allow",
			Params: map[string]any{"on": true}}},
	}, {
		Name: "Emulator toolchain", Note: "never found on PATH",
		Rows: []state.SetupRow{{
			Name: "radioserver", State: string(state.SetupMissing),
			What: "the SX1262 model both emulators reach over a socket",
			Cost: "about 41.0 kB to download, once", Do: "fetch it",
			Verb:   "resource.fetch",
			Params: map[string]any{"name": "radioserver", "kind": "toolchain"}}, {
			Name: "renode", State: string(state.SetupBlocked),
			What: "the emulator the nRF52 boards are started under",
			Do:   "no macOS Intel package is published"}},
	}}

	nodes := &nodeview.Panel{}
	nodes.OnSelect = func(string) { r.do("nodes.select", nil) }
	nv := &nodeview.ViewPanel{}
	// The build list is an overlay: its buttons do not exist until a firmware
	// cell has been clicked, and auditing them shut only proves they are shut.
	nv.OpenBuildPicker(auditBuilds, "Abernethy Repeater")
	nv.OnAction = func(a string, n string) { r.do(a, n) }
	nv.OnFirmware = func(n string, b nodeview.BuildChoice) { r.do("node.set_firmware", b.Version) }
	nw := &nodeview.WindowPanel{Node: "Abernethy Repeater"}
	// The bring-up window, on a node with a board: without one it correctly
	// says there is no wiring to check, and auditing that proves only the
	// guard. Its pop-out button opens a window rather than firing a verb, so
	// the audit answers it the way it answers the file dialog.
	bu := &boardview.Panel{Node: "Abernethy Repeater", OnDo: func(v string, _ any) { r.do(v, "") }}
	bu.OnPopScreen = func(n string) { r.do("ui.popout", n) }
	// The picture button opens the platform's dialog before it reaches a verb,
	// so the audit answers it the way it answers the file dialog itself.
	bu.OnSaveShot = func(n, suggested string) { r.do("board.screenshot", n) }
	snapWithBoard := uitest.Snapshot()
	// The node's own row, given a board - not a second row for the same name.
	// Appending one put the board on a duplicate the lookup never reached, and
	// the window correctly drew "no wiring to check" for a node that had some.
	for i := range snapWithBoard.Stats {
		if snapWithBoard.Stats[i].Name != "Abernethy Repeater" {
			continue
		}
		snapWithBoard.Stats[i].Board = "LilyGo_TDeck"
		snapWithBoard.Stats[i].Backend = "emulated"
		snapWithBoard.Stats[i].IRQReads = 12
		snapWithBoard.Stats[i].Radio = state.RadioState{Reported: true,
			Boosted: true, GainReg: 0x96, TxPowerDBm: 22, SF: 10, CR: 5,
			FreqHz: 869618000, BandwidthHz: 250000, IRQMask: 2}
	}
	snapWithConsole := uitest.Snapshot()
	// A card slot on the node the window is about, so the Hardware tab draws
	// its card controls: with no slot it correctly offers none, and auditing
	// that would only prove the guard works.
	for i := range snapWithConsole.Nodes {
		if snapWithConsole.Nodes[i].Name == "Abernethy Repeater" {
			snapWithConsole.Nodes[i].CardSlot = true
			snapWithConsole.Nodes[i].CardFitted = true
			// Handed a card of its own rather than using the node's, so the
			// control that puts it back is drawn: with its own it correctly
			// offers none.
			snapWithConsole.Nodes[i].CardFile = "/srv/cards/shared.img"
			snapWithConsole.Nodes[i].CardShared = true
		}
	}
	snapWithConsole.ConsoleNode = "Abernethy Repeater"
	snapWithConsole.Console = []string{"   0.000  > advert"}
	nw.OnCommand = func(n, l string) { r.do("console.type", l) }
	nw.OnAction = func(a, n string) { r.do(a, n) }
	nw.OnServe = func(node, kind string) { r.do("bench.serve", kind) }
	nw.Companion.OnCLI = func(n, l string) { r.do("console.cli", l) }
	nw.Companion.OnDo = func(verb string, _ any) { r.do(verb, "") }
	nw.OnDo = nw.Companion.OnDo
	cfgSets := &settings{}
	cfg := &configPanel{do: r.do, sets: cfgSets}
	// Choosing is the shell's overlay; what the audit can ask is whether
	// pressing the dropdown reaches the chooser at all.
	cfg.choose = func(title string, _ []string, _ func(string)) { r.do("ui.choose", title) }
	snapGPU := uitest.Snapshot()
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
	snapWithCompanion := uitest.Snapshot()
	for i := range snapWithCompanion.Nodes {
		snapWithCompanion.Nodes[i].Selected = snapWithCompanion.Nodes[i].Kind == "companion"
	}

	// One build's own window. It draws from the library row it was opened on,
	// so the snapshot needs that row: with no row it correctly says the build
	// has gone, and auditing that would only prove the guard works.
	fwWin := &firmwareWindowPanel{
		role: "companion_radio_usb", version: "mesh-rs", board: "LilyGo_TDeck",
	}
	fwWin.OnDo = func(verb string, _ any) { r.do(verb, "") }
	snapWithBuild := uitest.Snapshot()
	snapWithBuild.Library = []state.FirmwareRow{{
		Role: "companion_radio_usb", Version: "mesh-rs", Board: "LilyGo_TDeck",
		OnDisk: true, Bytes: 3 << 20, InUse: 1,
		Path:  "/cache/board/LilyGo_TDeck/companion_radio_usb@mesh-rs.bin",
		Facts: emulated.ImageFacts{Kind: "whole flash image", Bootable: true, FlashMB: 16},
	}}

	// One log in a window of its own. Its source buttons switch what the
	// window is rather than opening more of them, so they reach no verb - the
	// same excuse the tab's own do not need, because there they change what
	// the session is watching.
	logWin := &nodeview.OutputWindowPanel{Node: "Abernethy Repeater"}
	logWin.OnDo = func(verb string, _ any) { r.do(verb, "") }
	snapWithLog := uitest.Snapshot()
	snapWithLog.Outputs = []state.OutputPane{{
		Node: "Abernethy Repeater", Source: "serial", Total: 2,
		Lines: []string{"ets Jul 29 2019", "[BOOT] radio ok"},
		Path:  "/cache/nodefs/Abernethy Repeater/console.log",
	}}

	targets := []target{
		{"Nodes running", nv, nv.Draw, nil,
			// Choosing a build closes the list, so it is reopened before each.
			func() { nv.ReopenBuildPicker("Abernethy Repeater") }, nil, buildSkips()},
		// apply and delete are only drawn once there is something to apply
		// and once the first press has asked, so the panel is put into both
		// states before each press rather than being audited shut.
		{"Firmware window", fwWin, fwWin.auditDraw, snapWithBuild,
			func() {
				// Back onto the build, because applying a rename moves the
				// window onto the new name and the next press would land on
				// a window saying the build has gone.
				fwWin.role, fwWin.version, fwWin.board =
					"companion_radio_usb", "mesh-rs", "LilyGo_TDeck"
				fwWin.coproc.Bool.Value = true
				fwWin.confirm = true
			}, nil,
			map[string]string{
				"revert":   "puts the editors back rather than reaching a verb",
				"boardBtn": "opens the board list rather than reaching a verb",
				"coproc":   "changes what apply would send rather than reaching a verb",
				"card":     "changes what apply would send rather than reaching a verb",
				"name":     "changes what apply would send rather than reaching a verb",
				"notes":    "changes what apply would send rather than reaching a verb",
			}},
		{"Output window", logWin, logWin.Draw, snapWithLog, nil, nil,
			map[string]string{
				"out.pauseBtn":   "changes what this pane draws rather than reaching a verb",
				"out.popBtn":     "not drawn in a window that is already popped out",
				"out.srcBtns[0]": "switches what this window is rather than reaching a verb",
				"out.srcBtns[1]": "switches what this window is rather than reaching a verb",
				"out.srcBtns[2]": "switches what this window is rather than reaching a verb",
				"out.srcBtns[3]": "switches what this window is rather than reaching a verb",
			}},
		{"Node window", nw, nw.AuditDraw, snapWithConsole,
			// The tab row is above everything, so a pointer moving down the
			// panel leaves the console before it reaches the send button.
			func() { nw.Tab = 0 }, nil, nodeWindowSkips()},
		{"Node window: companion", &nw.Companion, nw.Companion.AuditDraw, nil, nil, nil, nil},
		{"Board view", bu, bu.AuditDraw, snapWithBoard, nil, nil, nil},
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
		{"Setup", setP, setP.auditDraw, snapWithSetup, nil, nil, nil},
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

// nodeWindowSkips is what the node window is not expected to answer.
//
// It carries the build list's own skips, because the window grows the same
// control the Nodes running panel does and a list you have to scroll is not a
// list that is broken.
func nodeWindowSkips() map[string]string {
	skip := map[string]string{
		// The companion tab belongs to a companion, and this node is a
		// repeater. Its controls are audited on their own below.
		"Companion.msg": "companion only", "Companion.scope": "companion only",
		"Companion.cmd": "companion only", "Companion.sendMsg": "companion only",
		"Companion.applyScope": "companion only", "Companion.advertBtn": "companion only",
		"Companion.refreshBtn": "companion only", "Companion.runCmd": "companion only",
		"Companion.connectBtn": "companion only", "Companion.release": "companion only",
		"Companion.serveBtn": "companion only", "Companion.stopServeBtn": "companion only",
		"Companion.dropBtn": "companion only", "Companion.tcpChip": "companion only",
		"Companion.newChan": "companion only", "Companion.addChan": "companion only",
		"Companion.ptyChip": "companion only",
		"Companion.setName": "companion only", "Companion.setFreq": "companion only",
		"Companion.setBW": "companion only", "Companion.setSF": "companion only",
		"Companion.setCR": "companion only", "Companion.setTx": "companion only",
		"Companion.applyRadio": "companion only",
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
		// Following the end of a file is a property of the pane, not of the
		// session: nothing outside this window needs to know, and a verb for
		// it would be a verb whose only caller is the button beside it. Held
		// instead by TestPausingStopsTheOutputPaneChasingTheEnd, which
		// presses it and checks what it changed.
		"out.pauseBtn": "changes what this pane draws rather than reaching a verb",
		"changeFw":     "opens the build list rather than reaching a verb",
		"pick.cancel":  "drawn in the overlay, which the flat audit layout has not got",
		"pick.filter":  "drawn in the overlay, which the flat audit layout has not got",
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
func auditBuilds() []nodeview.BuildChoice {
	return []nodeview.BuildChoice{
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
