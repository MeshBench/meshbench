// Every panel the workbench offers, and the startup flags that open one on
// something.
//
// Lifted out of Run, which had grown past a thousand lines with this block the
// largest thing in it. It is a list by nature - add a panel, wire its action
// bar, say whether it can be popped out - and a list belongs somewhere it can
// be read end to end, rather than sandwiched between the flag parsing and the
// window machinery.
package workbench

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"gioui.org/layout"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// panelDeps is what the panel list needs from Run.
//
// A struct rather than thirty parameters. Everything in it is built by Run and
// borrowed here, never owned: this is wiring, not a component.
type panelDeps struct {
	sh  *shell.Shell
	st  *state.Store
	ctx context.Context
	mv  *comp.MapView
	// wbUI, wins and mapTop are the window and map machinery a panel reaches
	// to open a node window, post a prompt, or read the map's own toolbar.
	wbUI   *workbenchUI
	wins   *windows
	mapTop *mapTools
	nodes  *nodesPanel
	// do runs a verb and puts any failure in the status bar. withControls
	// puts an action bar above a panel's body, chooserIn posts a chooser in
	// whichever window the panel is currently in, and openPacket opens the
	// packet view on an id.
	do           Do
	withControls func(ctrl, body func(*theme.Theme, layout.Context, *state.Snapshot) layout.Dimensions) func(*theme.Theme, layout.Context, *state.Snapshot) layout.Dimensions
	chooserIn    func(panel string) func(string, []string, func(string))
	openPacket   func(id uint64)
	// The action bars, one per panel that has one.
	fleetCtl  *fleetControls
	schedCtl  *scheduleControls
	importCtl *importControls
	boundCtl  *boundaryControls
	validCtl  *validateControls
	planCtl   *planningControls
	benchCtl  *benchControls
	feedCtl   *feedControls
	sweepCtl  *sweepControls
	provCtl   *provisioningControls
	// The flags that ask for a panel to be open, or open on something, at
	// startup - so a screenshot can be driven rather than clicked.
	cfgSection, licSection            *string
	filterFlag, importFlag            *string
	nodeWinFlag, provFlag, openFwFlag *string
	openMenuFlag                      *string
	packetTabFlag, nodeTabFlag        *int
}

// addPanels registers every panel, in the order the shell shows them.
//
// It hands back the Configuration panel because Run keeps wiring it after
// this returns - the settings page and the menu both reach into it.
func addPanels(d panelDeps) *configPanel {
	d.sh.Add(&shell.Panel{Name: "Map", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			d.wbUI.applyCamera()
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return d.mapTop.Draw(t, gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return d.mv.Layout(t, gtx, s)
				}),
			)
		}})
	d.sh.Add(&shell.Panel{Name: "Nodes", Windowable: true, Draw: d.nodes.Draw})
	// The Inspector is the events panel's light form, scoped to the
	// selection: what this node has said and heard, not a form about it.
	insp := &eventsPanel{compact: true, forNode: true, OnOpenPacket: d.openPacket}
	d.sh.Add(&shell.Panel{Name: "Inspector", Windowable: true, Draw: insp.Draw})
	pkt := &packetPanel{do: d.do}
	d.sh.Add(&shell.Panel{Name: "Packet", Windowable: true, Draw: pkt.Draw})
	events := &eventsPanel{OnOpenPacket: d.openPacket}
	scores := &scorePanel{}
	d.sh.Add(&shell.Panel{Name: "Events", Windowable: true, Draw: events.Draw})
	d.sh.Add(&shell.Panel{Name: "Scoreboard", Windowable: true, Draw: scores.Draw})
	logs := &logsPanel{do: d.do}
	d.sh.Add(&shell.Panel{Name: "Logs", Windowable: true, Draw: logs.Draw})
	tl := &comp.Timeline{}
	d.sh.Add(&shell.Panel{Name: "Packet timeline", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return tl.Layout(t, gtx, s)
		}})
	d.sh.Add(&shell.Panel{Name: "Budget", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return comp.Budget{}.Layout(t, gtx, s)
		}})
	d.sh.Add(&shell.Panel{Name: "Matrix", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return comp.Matrix{}.Layout(t, gtx, s)
		}})
	d.sh.Add(&shell.Panel{Name: "Energy", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return comp.Energy{}.Layout(t, gtx, s)
		}})
	wf := &comp.Waterfall{}
	d.sh.Add(&shell.Panel{Name: "Waterfall", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return wf.Layout(t, gtx, s)
		}})
	feed := &feedPanel{}
	feed.OnPull = func() {
		go func() { _, _ = d.st.Do(d.ctx, "feed.pull", *d.importFlag) }()
	}
	d.sh.Add(&shell.Panel{Name: "Live feed", Windowable: true,
		Draw: d.withControls(d.feedCtl.Draw, feed.Draw)})
	d.sh.Add(&shell.Panel{Name: "Validate", Windowable: true,
		Draw: d.withControls(d.validCtl.Draw, (&validatePanel{}).Draw)})
	imp := &importPanel{}
	imp.OnFetch = func(url string) {
		go func() { _, _ = d.st.Do(d.ctx, "import.describe", url) }()
	}
	d.sh.Add(&shell.Panel{Name: "Import", Windowable: true,
		Draw: d.withControls(d.importCtl.Draw, imp.Draw)})
	plan := &planPanel{}
	plan.OnRun = func() {
		go func() { _, _ = d.st.Do(d.ctx, "plan.routes", nil) }()
	}
	d.sh.Add(&shell.Panel{Name: "Planning", Windowable: true,
		Draw: d.withControls(d.planCtl.Draw, plan.Draw)})
	cmpP := &comparePanel{}
	cmpP.OnSave = func() {
		go func() { _, _ = d.st.Do(d.ctx, "run.save", "run") }()
	}
	d.sh.Add(&shell.Panel{Name: "Compare", Windowable: true, Draw: cmpP.Draw})
	cfg := &configPanel{do: d.do, choose: d.chooserIn("Configuration")}
	if *d.cfgSection != "" {
		cfg.Open(*d.cfgSection)
	}
	logp := &logPanel{}
	d.sh.Add(&shell.Panel{Name: "Provisioning", Windowable: true,
		Draw: d.withControls(d.provCtl.Draw, shell.EmptyPanel("Provisioning",
			"what every node is told when it starts").Draw)})
	d.sh.Add(&shell.Panel{Name: "Configuration", Windowable: true, Draw: cfg.Draw})
	d.sh.Add(&shell.Panel{Name: "Experiment log", Windowable: true, Draw: logp.Draw})
	lic := &licPanel{}
	// A chip is a click, and a click cannot be captured; the flag is how a
	// screenshot of one section gets taken.
	lic.openAt = *d.licSection
	d.sh.Add(&shell.Panel{Name: "Licences", Windowable: true, Draw: lic.Draw})
	nv := &nodeViewPanel{}
	nv.OnAction = func(action, node string) {
		go func() {
			if action == "d.nodes.stats" {
				_, _ = d.st.Do(d.ctx, action, nil)
				return
			}
			_, _ = d.st.Do(d.ctx, action, node)
		}()
	}
	nv.OnFirmware = func(node, version string) {
		go func() {
			_, _ = d.st.Do(d.ctx, "node.set_firmware",
				map[string]any{"node": node, "version": version})
		}()
	}
	if *d.filterFlag != "" {
		nv.SetFilter(*d.filterFlag)
	}
	openOnTab = nodeTab(*d.nodeTabFlag)
	packetOpenOnTab = *d.packetTabFlag
	if *d.nodeWinFlag != "" {
		go func() {
			time.Sleep(4 * time.Second)
			if _, err := d.st.Do(d.ctx, "node.window", *d.nodeWinFlag); err != nil {
				fmt.Fprintln(os.Stderr, "node.window:", err)
			}
		}()
	}
	if *d.provFlag != "" {
		go func() {
			if _, err := d.st.Do(d.ctx, "node.provisioning", *d.provFlag); err != nil {
				fmt.Fprintln(os.Stderr, "provisioning:", err)
			}
		}()
	}
	if *d.openFwFlag != "" {
		nv.OpenFirmware(*d.openFwFlag)
	}
	if *d.openMenuFlag != "" {
		go func() {
			// After the first snapshot, so the menu knows whether the
			// node is running and can offer stop rather than start.
			time.Sleep(30 * time.Second)
			nv.OpenMenu(*d.openMenuFlag, d.st.Snapshot())
		}()
	}
	d.sh.Add(&shell.Panel{Name: "Nodes running", Windowable: true, Draw: nv.Draw})
	// Keep the node view live while somebody is looking at it.
	//
	// Sampling costs a /proc read per node, so it is driven by the panel
	// having drawn rather than by a timer that runs whether or not the view
	// is open - and it stops as soon as the panel does.
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-d.ctx.Done():
				return
			case <-t.C:
				if nv.watched.Swap(false) {
					_, _ = d.st.Do(d.ctx, "d.nodes.stats", nil)
				}
			}
		}
	}()
	fleet := &fleetPanel{}
	bounds := &boundaryPanel{}
	tls := &timelinesPanel{}
	d.sh.Add(&shell.Panel{Name: "Fleet", Windowable: true,
		Draw: d.withControls(d.fleetCtl.Draw, fleet.Draw)})
	d.sh.Add(&shell.Panel{Name: "Boundary", Windowable: true,
		Draw: d.withControls(d.boundCtl.Draw, bounds.Draw)})
	d.sh.Add(&shell.Panel{Name: "Timelines", Windowable: true, Draw: tls.Draw})
	bench := &benchPanel{}
	bench.OnAction = func(action, node string) {
		go func() {
			switch action {
			case "serve.tcp":
				_, _ = d.st.Do(d.ctx, "bench.serve",
					map[string]any{"node": node, "kind": "tcp"})
			case "serve.serial":
				_, _ = d.st.Do(d.ctx, "bench.serve",
					map[string]any{"node": node, "kind": "serial"})
			default:
				_, _ = d.st.Do(d.ctx, action, nil)
			}
		}()
	}
	d.sh.Add(&shell.Panel{Name: "Companion bench", Windowable: true,
		Draw: d.withControls(d.benchCtl.Draw, bench.Draw)})
	sched := &schedulePanel{}
	console := &consolePanel{}
	d.sh.Add(&shell.Panel{Name: "Schedule", Windowable: true,
		Draw: d.withControls(d.schedCtl.Draw, sched.Draw)})
	d.sh.Add(&shell.Panel{Name: "Link", Windowable: true, Draw: linkPanel{}.Draw})
	d.sh.Add(&shell.Panel{Name: "Console", Windowable: true, Draw: console.Draw})
	fw := &firmwarePanel{choose: d.chooserIn("Firmware")}
	// The library asks for itself, and asks again after anything that changes
	// it. A panel that reads the cache directly cannot know when a download
	// has landed.
	fw.Refresh = func() {
		go func() { _, _ = d.st.Do(d.ctx, "firmware.library", nil) }()
	}
	fw.OnAction = func(verb string, params map[string]any) {
		go func() {
			if _, err := d.st.Do(d.ctx, verb, params); err != nil {
				_, _ = d.st.Do(d.ctx, "ui.said", verb+": "+err.Error())
			}
			_, _ = d.st.Do(d.ctx, "firmware.library", nil)
		}()
	}
	// Importing needs a path and a role, which a button cannot carry: ask for
	// both through the shell, then refresh so the new build appears.
	fw.OnImport = func() {
		// Both questions go to the Firmware panel's own window: asked from a
		// pop-out, answered in the pop-out.
		fwAsk := d.wins.promptFor("Firmware", &d.sh.Ask)
		fwAsk.Post(func(ask *shell.Prompt) {
			ask.OpenPath("Import a build from", "path to a binary", "",
				shell.PathAsk{Kind: shell.PathFile}, func(path string) {
					if strings.TrimSpace(path) == "" {
						return
					}
					fwAsk.Post(func(ask *shell.Prompt) {
						ask.Choose("Import it as which role?", "filter", []string{
							"simple_repeater", "advanced_repeater", "companion_radio",
							"simple_room_server",
						}, func(role string) {
							go func() {
								if _, err := d.st.Do(d.ctx, "firmware.import",
									map[string]any{"path": path, "role": role}); err != nil {
									_, _ = d.st.Do(d.ctx, "ui.said", "import: "+err.Error())
								}
								_, _ = d.st.Do(d.ctx, "firmware.library", nil)
							}()
						})
					})
				})
		})
	}
	runs := &runsPanel{}
	d.sh.Add(&shell.Panel{Name: "Firmware", Windowable: true, Draw: fw.Draw})
	d.sh.Add(&shell.Panel{Name: "Runs", Windowable: true, Draw: runs.Draw})
	// Controls and results are two panels, not one.
	//
	// Together in a 340dp rail they made each other useless: the fields were
	// too narrow to read a version string in, and the table underneath got
	// whatever was left, which was nothing. The Bench view puts the controls in
	// a fixed column and gives the table the middle.
	d.sh.Add(&shell.Panel{Name: "Sweep", Windowable: true, Draw: d.sweepCtl.Draw})
	d.sh.Add(&shell.Panel{Name: "Results", Windowable: true, Draw: (&sweepResults{}).Draw})
	return cfg
}
