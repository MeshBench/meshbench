// Package workbench is the workbench: the shell, the state layer and the
// panels, wired together. This is the application - `meshbench workbench`
// lands here.
package workbench

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	fixturelib "github.com/MeshBench/meshbench/internal/app/fixture"
	"github.com/MeshBench/meshbench/internal/app/session"
	_ "github.com/MeshBench/meshbench/internal/app/session/domains"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/app/version"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/desktop"
	"github.com/MeshBench/meshbench/internal/ui/pick"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// Run is the whole application. It owns the process: it parses args, opens
// windows, and only returns when the last one closes.
func Run(args []string) {
	// A name, not a path: an installed copy has no fixtures in its working
	// directory, and this default has to open on a machine where the only
	// copy is the one inside the binary. A path still works and still wins.
	fixture := flag.String("fixture", "scotland-ireland-strict",
		"network to load: a name (see -list-fixtures) or a path to a .json")
	listFixtures := flag.Bool("list-fixtures", false, "list the built-in networks and exit")
	// The same flag headless has, and for the same reason. It was only on
	// headless, so a client launching a windowed run with a seed built a
	// command line the binary refused - exit 2, before the window appeared,
	// which reads as the workbench failing to start rather than as a flag it
	// has not got.
	seedFlag := flag.Uint("seed", 0, "override the scenario's seed")
	modeFlag := flag.String("theme", "dark", "dark or light")
	viewFlag := flag.String("view", "plan", "which view to open")
	fpsFlag := flag.Bool("fps", false, "report frames per second to stderr and /tmp/wb2-fps.log")
	panelFlag := flag.String("panel", "", "draw only this panel, filling the window")
	memFlag := flag.String("memprofile", "", "write a heap profile here on exit")
	profFlag := flag.String("cpuprofile", "", "write a CPU profile here")
	playFlag := flag.Bool("play", false, "start the simulation immediately")
	injectFlag := flag.String("inject", "", "originate a packet at this node once running")
	openFwFlag := flag.String("open-firmware", "", "open this node's firmware list at startup")
	openMenuFlag := flag.String("node-menu", "", "open this node's context menu at startup")
	provFlag := flag.String("provisioning", "", "show what this node is told at boot, at startup")
	nodeWinFlag := flag.String("node-window", "", "open this node's own window at startup")
	filterFlag := flag.String("filter", "", "preset the node view's search box, so a filtered table can be captured")
	popFlag := flag.String("pop-out", "", "open this panel in its own window at startup")
	importFlag := flag.String("import", "", "describe an import from this CoreScope URL at startup")
	planFlag := flag.String("plan", "", "plan between the selected node and this one at startup")
	sweepFlag := flag.Bool("sweep", false, "run the default sweep at startup")
	saveRunFlag := flag.String("save-run", "", "save a run record under this name, then keep running")
	shadeFlag := flag.Bool("terrain", false, "shade the relief at startup")
	menuFlag := flag.String("menu", "", "fire this menu action at startup, so what it opens can be captured")
	cfgSection := flag.String("config-section", "", "open the Configuration page on this section")
	licSection := flag.String("licence-section", "",
		"scope the Licences panel to one section: forks, bundled, golibs, runtime, data")
	dropFlag := flag.String("drop-menu", "", "open this menu's dropdown at startup, so it can be captured")
	layersFlag := flag.String("layers", "", "switch these map layers on at startup, comma separated")
	lookFlag := flag.String("look", "", "start the camera at lat,lon,zoom - a capture cannot drag the map")
	packetTabFlag := flag.Int("packet-tab", 0,
		"which tab the packet window opens on: 0 dissection, 1 journey "+
			"(the propagation graph), 2 reception ledger, 3 where it went")
	nodeTabFlag := flag.Int("node-tab", 0, "which tab a node window opens on: "+
		"0 console, 1 companion, 2 SDR, 3 settings, 4 radio, 5 stats, "+
		"6 activity, 7 connect, 8 hardware, 9 output")
	coverFlag := flag.String("coverage", "",
		"compute and show coverage from this node at startup")
	energyFlag := flag.Bool("energy", false, "run the site study for the selected node at startup")
	captureFlag := flag.String("capture", "",
		"capture the waterfall at this node once the run has traffic")
	injectEvery := flag.Duration("inject-every", 0,
		"keep originating at that node this often; for looking at the traffic layer")
	unwatchedFlag := flag.Bool("unverified-wiring", false,
		"run boards whose emulation wiring nobody has watched boot yet")
	quitFlag := flag.Duration("quit-after", 0, "exit after this long; 0 runs until closed")
	socketFlag := flag.String("control-socket", "",
		"where the control socket answers: a path for a unix socket, or "+
			"\"tcp\" for loopback with a token (the default on Windows, which "+
			"has no unix socket a Python client can reach). "+
			"MESHBENCH_CONTROL_SOCKET does the same, and two workbenches need two")
	versionFlag := flag.Bool("version", false, "print the version and exit")
	updateFeedFlag := flag.String("update-feed", "", "ask this URL whether a "+
		"newer release exists instead of the published release feed")
	updateNowFlag := flag.Bool("update-check", false, "ask whether a newer "+
		"release exists shortly after startup, whatever the schedule says")
	_ = flag.CommandLine.Parse(args)
	if *versionFlag {
		fmt.Println("MeshBench", version.Detail())
		return
	}
	if *listFixtures {
		for _, n := range fixturelib.Embedded() {
			fmt.Println(n)
		}
		return
	}

	// Before anything is constructed: see startupfixture.go.
	refuseUnknownFixture(*fixture)

	// The same construction the headless command uses, so the two modes
	// cannot become two ways of building a session that drift. What this one
	// does afterwards that headless does not is attach a UI.
	st, sm := session.Boot(session.Options{
		UnverifiedWiring: *unwatchedFlag, UpdateFeed: *updateFeedFlag})
	// Every status line, timestamped and kept in full - not just the last
	// twenty the strip at the bottom shows. Set before Run starts: nothing
	// else touches World before then, so there is nothing to race. A run
	// that goes quiet for reasons nobody was watching for is the thing this
	// is for, so it has to already be running before anything goes wrong.
	if logPath, err := openSessionLog(st, sm); err != nil {
		fmt.Fprintln(os.Stderr, "session log:", err)
	} else if logPath != "" {
		fmt.Fprintln(os.Stderr, "session log:", logPath)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)
	// Put the loaded settings in the snapshot, so the Configuration page
	// opens saying what is actually in force rather than its zero values.
	go func() {
		_, _ = st.Do(ctx, "terrain.cache", nil)
		_, _ = st.Do(ctx, "gpu.state", nil)
		// What firmware is on this machine, before anything asks to choose
		// from it. The library was filled only by opening the Firmware panel,
		// so the Bench offered an empty list of arms and said the machine held
		// no repeater builds while holding several - a listing of one
		// directory, deferred far past the point it was needed.
		_, _ = st.Do(ctx, "firmware.installed", nil)
	}()

	// Opened on a worker, not here. Building the engine for a national
	// fixture takes a moment, and doing it before the window exists is an
	// application that has not appeared yet - which is indistinguishable, to
	// whoever started it, from one that has crashed.
	go startupNetwork{
		st: st, ctx: ctx, fixture: *fixture, seed: *seedFlag,
		play: *playFlag, inject: *injectFlag, injectEvery: *injectEvery,
	}.run()

	// The control socket, before any window: a script that drives the
	// workbench should not have to wait for a frame.
	if srv, err := session.ServeControlAt(ctx, st, *socketFlag); err != nil {
		fmt.Fprintln(os.Stderr, "control socket:", err)
	} else {
		defer func() { _ = srv.Close() }()
	}

	// Close the nodes when asked to stop. See shutdown.go.
	onSignal(ctx, cancel, sm)

	sh := shell.New()
	// How the shell opens a file dialog. Wired here rather than inside the
	// shell so that package keeps knowing nothing about which library does
	// it - and so a test can replace it.
	shell.Browse = func(title, start string, what shell.PathAsk) (string, error) {
		var f []pick.Filter
		if len(what.Extensions) > 0 {
			f = append(f, pick.Filter{Name: what.FilterName, Extensions: what.Extensions})
		}
		return pick.Open(title, start, pick.Kind(what.Kind), f...)
	}
	wins := newWindows()
	// One dispatcher for every action panel. A verb that fails says so in the
	// status bar rather than silently doing nothing, which is what a button
	// with no feedback looks like from the other side of the screen.
	do := func(verb string, params any) {
		go func() {
			if _, err := st.Do(ctx, verb, params); err != nil {
				_, _ = st.Do(ctx, "ui.said", verb+": "+err.Error())
			}
		}()
	}
	// withControls puts an action bar above a panel's own body.
	withControls := func(ctrl func(*theme.Theme, layout.Context, *state.Snapshot) layout.Dimensions,
		body func(*theme.Theme, layout.Context, *state.Snapshot) layout.Dimensions,
	) func(*theme.Theme, layout.Context, *state.Snapshot) layout.Dimensions {
		return func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ctrl(t, gtx, s)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return body(t, gtx, s)
				}),
			)
		}
	}
	// One chooser for every dropdown: a prompt overlay is the single way
	// anything here picks from a list. Which window's overlay depends on
	// where the panel is right now - a question asked in a popped-out panel
	// belongs in that window, not behind it in the main one.
	chooserIn := func(panel string) func(string, []string, func(string)) {
		return func(title string, opts []string, pick func(string)) {
			wins.promptFor(panel, &sh.Ask).Post(func(ask *shell.Prompt) {
				ask.Choose(title, "filter", opts, pick)
			})
		}
	}
	// askerIn is the same, for a value that has to be typed because no list
	// could hold it - a firmware setting this build has never heard of.
	askerIn := func(panel string) func(string, string, string, func(string)) {
		return func(title, hint, initial string, got func(string)) {
			wins.promptFor(panel, &sh.Ask).Post(func(ask *shell.Prompt) {
				ask.Open(title, hint, initial, got)
			})
		}
	}
	// chooser keeps the old shape for panels that never pop out.
	chooser := chooserIn("")
	fleetCtl := &fleetControls{do: do, choose: chooserIn("Fleet")}
	schedCtl := &scheduleControls{do: do}
	importCtl := &importControls{do: do}
	boundCtl := &boundaryControls{do: do}
	validCtl := &validateControls{do: do}
	planCtl := &planningControls{do: do}
	benchCtl := &benchControls{do: do}
	feedCtl := &feedControls{do: do}
	sweepCtl := &sweepControls{do: do,
		choose: chooserIn("Sweep"), askText: askerIn("Sweep")}
	provCtl := &provisioningControls{do: do}
	// openPacket dissects a clicked transmission and puts the packet view in
	// its own window, which is where wb1 kept it.
	openPacket := func(id uint64) {
		go func() {
			if _, err := st.Do(ctx, "packet.open", map[string]any{"id": float64(id)}); err != nil {
				_, _ = st.Do(ctx, "ui.said", "packet: "+err.Error())
			}
		}()
		sh.OnPopOut("Packet")
	}

	mv := &comp.MapView{}
	// The buildings layer reads straight from the loaded environment: a
	// city of polygons has no business in the world snapshot.
	mv.BuildingsIn = sm.BuildingsIn
	wbUI := &workbenchUI{sh: sh, sim: sm, mv: mv, store: st,
		nodes: newNodeWindows(), builds: newFirmwareWindows(),
		logs: newOutputWindows()}
	callbacks{
		wbUI: wbUI, mv: mv, st: st, ctx: ctx, sm: sm, openPacket: openPacket,
		chooser: chooser, do: do,
	}.wire()

	startupActions{
		st: st, ctx: ctx, mv: mv, sh: sh,
		energyFlag: energyFlag, saveRunFlag: saveRunFlag, importFlag: importFlag,
		planFlag: planFlag, captureFlag: captureFlag, coverFlag: coverFlag,
		layersFlag: layersFlag, lookFlag: lookFlag, sweepFlag: sweepFlag, shadeFlag: shadeFlag,
	}.run()

	nodes := &nodesPanel{}
	nodes.OnSelect = func(name string) {
		go func() { _, _ = st.Do(ctx, "nodes.select", name) }()
	}
	mapTop := &mapTools{mv: mv}
	wireMapTools(mv, mapTop, sh, st, ctx)
	cfg := addPanels(panelDeps{
		sh: sh, st: st, ctx: ctx, mv: mv, wbUI: wbUI, wins: wins, mapTop: mapTop, nodes: nodes,
		do: do, withControls: withControls, chooserIn: chooserIn, openPacket: openPacket,
		fleetCtl: fleetCtl, schedCtl: schedCtl, importCtl: importCtl, boundCtl: boundCtl,
		validCtl: validCtl, planCtl: planCtl, benchCtl: benchCtl, feedCtl: feedCtl,
		sweepCtl: sweepCtl, provCtl: provCtl,
		cfgSection: cfgSection, licSection: licSection, filterFlag: filterFlag,
		importFlag: importFlag, nodeWinFlag: nodeWinFlag, provFlag: provFlag,
		openFwFlag: openFwFlag, openMenuFlag: openMenuFlag,
		packetTabFlag: packetTabFlag, nodeTabFlag: nodeTabFlag,
	})
	// After the panels are registered, because it opens one of them. Not when
	// a single panel fills the window: that is a capture, and a page docking
	// itself over the one asked for would be the last thing a screenshot
	// wanted.
	if *panelFlag == "" {
		openSetupIfNotReady(ctx, st)
	}
	scheduleUpdateCheck(ctx, st, sm, *updateNowFlag)
	switch *viewFlag {
	case "run":
		sh.View = shell.Run
	case "debug":
		sh.View = shell.Debug
	// "verify" as well as "validate": the view was renamed, and a script or a
	// saved command written against the old name should not stop working
	// because a caption improved.
	case "validate", "verify":
		sh.View = shell.Validate
	case "bench":
		sh.View = shell.Bench
	case "app":
		sh.View = shell.App
	}

	sh2 := text.NewShaper(text.WithCollection(brandFaces()))
	mode := theme.Dark
	if *modeFlag == "light" {
		mode = theme.Light
	}
	sets := newSettings(mode)
	th := theme.New(mode, theme.Default, sh2)
	thGen := uint64(1)

	var meter *fpsMeter
	if *fpsFlag {
		what := "shell"
		if *panelFlag != "" {
			what = *panelFlag
		}
		meter = newFPSMeter(fmt.Sprintf("%-9s", what), "/tmp/wb2-fps.log")
	}
	if *profFlag != "" {
		f, err := os.Create(*profFlag)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer pprof.StopCPUProfile()
	}
	if *quitFlag > 0 {
		go func() {
			time.Sleep(*quitFlag)
			pprof.StopCPUProfile()
			if *memFlag != "" {
				// After a GC, so what is reported is what is retained rather
				// than what merely has not been collected yet.
				runtime.GC()
				if f, err := os.Create(*memFlag); err == nil {
					_ = pprof.Lookup("heap").WriteTo(f, 0)
					_ = f.Close()
				}
			}
			os.Exit(0)
		}()
	}

	menuBar{sh: sh, sets: sets, cfg: cfg, dropFlag: dropFlag,
		st: st, ctx: ctx, nodes: nodes, wins: wins,
		chooser: chooser, menuFlag: menuFlag,
		onShown: rasterMenuIntercept(mv, st, ctx)}.build()

	// -menu fires one at startup, so what it opens can be captured.
	if *menuFlag != "" {
		sh.OnMenu(*menuFlag)
	}
	wbUI.newTheme = func() *theme.Theme {
		m, d, _ := sets.get()
		return theme.New(m, d,
			text.NewShaper(text.WithCollection(brandFaces())))
	}
	wbUI.dock = func(name string) { wins.dock(name) }
	wbUI.closeWin = func(name string) error { return wins.close(name) }
	wbUI.scale = sets.getScale
	wbUI.setScale = sets.setScale
	sh.PoppedOut = wins.has
	sh.OnPopOut = func(name string) {
		already := wins.has(name)
		wins.popOut(name, sh, func() *theme.Theme {
			// A shaper of its own, per window, and the current settings so a
			// window opened after a theme change does not open in the old one.
			m, d, _ := sets.get()
			return theme.New(m, d,
				text.NewShaper(text.WithCollection(brandFaces())))
		}, st)
		// Said, with the way back in it. A window that opens behind this one
		// is a panel that has apparently vanished, and right-clicking a tab
		// to send it out is a gesture nothing on screen advertises.
		line := name + " is in its own window - Window, then dock every window back"
		if already {
			line = name + " is already in its own window; raised it"
		}
		do("ui.said", line)
	}
	if *popFlag != "" {
		// Scriptable, so that a window which only opens on a click is a
		// window nobody can check without a hand on the mouse.
		go func() {
			time.Sleep(3 * time.Second)
			sh.OnPopOut(*popFlag)
		}()
	}

	// A misspelled panel name used to draw the whole shell instead, which
	// looks like the flag was ignored rather than wrong - and a name with a
	// space in it is easy to split by accident on the way here.
	for _, f := range []struct{ flag, name string }{
		{"-panel", *panelFlag}, {"-pop-out", *popFlag},
	} {
		if f.name == "" {
			continue
		}
		if _, ok := sh.Panels[f.name]; !ok {
			var have []string
			for n := range sh.Panels {
				have = append(have, n)
			}
			sort.Strings(have)
			fmt.Fprintf(os.Stderr, "%s %q: no such panel. There is: %s\n",
				f.flag, f.name, strings.Join(have, ", "))
			os.Exit(2)
		}
	}

	go func() {
		// Before the window: Gio reads the cursor theme once, when it
		// makes one, and the desktop does not put it in the environment.
		desktop.MatchSystemCursor()
		w := new(app.Window)
		w.Option(app.Title("MeshBench workbench"), app.Size(unit.Dp(1500), unit.Dp(940)))
		var ops op.Ops
		for {
			switch e := w.Event().(type) {
			case app.DestroyEvent:
				cancel()
				os.Exit(0)
			case app.FrameEvent:
				gtx := app.NewContext(&ops, e)
				began := time.Now()
				// Rebuilt only when something changed: a theme per frame
				// would throw away the shaper's font cache sixty times a
				// second to no purpose.
				if m, d, g := sets.get(); g != thGen {
					th = theme.New(m, d, sh2)
					thGen = g
				}
				// What the verbs asked for, applied here because this is the
				// goroutine allowed to do it, and before anything is drawn
				// from what they change.
				wbUI.applyDeferred()
				if p := sh.Panels[*panelFlag]; p != nil {
					comp.Fill(gtx, th.P.Ground)
					p.Draw(th, gtx, st.Snapshot())
				} else {
					// Applied here, on the goroutine that owns it.
					if v := pendingView.Swap(0); v > 0 {
						sh.View = shell.View(v - 1)
					}
					sh.Layout(th, gtx, st.Snapshot())
				}
				e.Frame(gtx.Ops)
				if meter != nil {
					meter.frame(time.Since(began))
				}
				// The renderer asks for the next frame; it does not drive the
				// simulation, which advances on the store's own ticker.
				w.Invalidate()
			}
		}
	}()
	app.Main()
}
