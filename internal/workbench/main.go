// Package workbench is the workbench: the shell, the state layer and the
// panels, wired together. This is the application - `meshcoresim workbench`
// and the standalone cmd/workbench2 both land here.
package workbench

import (
	"context"
	"flag"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/basemap"
	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/desktop"
	"github.com/MeshBench/meshbench/internal/gui/shell"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
	"github.com/MeshBench/meshbench/internal/session"
)

// Run is the whole application. It owns the process: it parses args, opens
// windows, and only returns when the last one closes.
func Run(args []string) {
	fixture := flag.String("fixture", "fixtures/fixture-scotland-ireland-strict.json",
		"network to load")
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
	dropFlag := flag.String("drop-menu", "", "open this menu's dropdown at startup, so it can be captured")
	layersFlag := flag.String("layers", "", "switch these map layers on at startup, comma separated")
	nodeTabFlag := flag.Int("node-tab", 0, "which tab a node window opens on: 0 console, 1 companion, 2 settings, 3 stats, 4 activity, 5 connect")
	coverFlag := flag.String("coverage", "",
		"compute and show coverage from this node at startup")
	energyFlag := flag.Bool("energy", false, "run the site study for the selected node at startup")
	captureFlag := flag.String("capture", "",
		"capture the waterfall at this node once the run has traffic")
	injectEvery := flag.Duration("inject-every", 0,
		"keep originating at that node this often; for looking at the traffic layer")
	quitFlag := flag.Duration("quit-after", 0, "exit after this long; 0 runs until closed")
	versionFlag := flag.Bool("version", false, "print the version and exit")
	_ = flag.CommandLine.Parse(args)
	if *versionFlag {
		fmt.Println("MeshBench", Version)
		return
	}

	st := state.New(10)
	sm := &session.Sim{}
	// What survived the last session: the GPU choice, the cache bound and
	// where the cache lives. Loaded here rather than in Register, so a test's
	// session never depends on this machine's own file.
	sm.LoadPrefs()
	session.Register(st, sm)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)
	// Put the loaded settings in the snapshot, so the Configuration page
	// opens saying what is actually in force rather than its zero values.
	go func() {
		_, _ = st.Do(ctx, "terrain.cache", nil)
		_, _ = st.Do(ctx, "gpu.state", nil)
	}()

	// Opened on a worker, not here. Building the engine for a national
	// fixture takes a moment, and doing it before the window exists is an
	// application that has not appeared yet - which is indistinguishable, to
	// whoever started it, from one that has crashed.
	go func() {
		if _, err := st.Do(ctx, "project.open", *fixture); err != nil {
			fmt.Fprintln(os.Stderr, "loading:", err)
		}
		if *playFlag {
			_, _ = st.Do(ctx, "sim.play", nil)
		}
		if *injectFlag != "" {
			_, _ = st.Do(ctx, "sim.inject", *injectFlag)
		}
		if *injectFlag != "" && *injectEvery > 0 {
			// Wall-clock rather than simulated time, because this exists to
			// put something on the map while a person is looking at it.
			go func() {
				t := time.NewTicker(*injectEvery)
				defer t.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-t.C:
						_, _ = st.Do(ctx, "sim.inject", *injectFlag)
					}
				}
			}()
		}
	}()

	// The control socket, before any window: a script that drives the
	// workbench should not have to wait for a frame.
	if srv, err := session.ServeControl(ctx, st); err != nil {
		fmt.Fprintln(os.Stderr, "control socket:", err)
	} else {
		defer func() { _ = srv.Close() }()
	}

	// Close the nodes when asked to stop. See shutdown.go.
	onSignal(ctx, cancel, sm)

	sh := shell.New()
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
	// One chooser for every dropdown: the shell's overlay is the single way
	// anything here picks from a list.
	chooser := func(title string, opts []string, pick func(string)) {
		sh.Ask.Post(func(ask *shell.Prompt) {
			ask.Choose(title, "filter", opts, pick)
		})
	}
	fleetCtl := &fleetControls{do: do, choose: chooser}
	schedCtl := &scheduleControls{do: do}
	importCtl := &importControls{do: do}
	boundCtl := &boundaryControls{do: do}
	validCtl := &validateControls{do: do}
	planCtl := &planningControls{do: do}
	benchCtl := &benchControls{do: do}
	feedCtl := &feedControls{do: do}
	sweepCtl := &sweepControls{do: do}
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
	wbUI := &workbenchUI{sh: sh, sim: sm, mv: mv, nodes: newNodeWindows(), store: st}
	wbUI.onCommand = func(node, line string) {
		go func() {
			_, _ = st.Do(ctx, "console.type",
				map[string]any{"node": node, "command": line})
		}()
	}
	wbUI.onAction = func(action, node string) {
		go func() { _, _ = st.Do(ctx, action, node) }()
	}
	wbUI.onCLI = func(node, line string) {
		go func() {
			if _, err := st.Do(ctx, "console.cli",
				map[string]any{"node": node, "command": line}); err != nil {
				_, _ = st.Do(ctx, "ui.said", err.Error())
			}
		}()
	}
	wbUI.onServe = func(node, kind string) {
		go func() {
			if _, err := st.Do(ctx, "bench.serve",
				map[string]any{"node": node, "kind": kind}); err != nil {
				_, _ = st.Do(ctx, "ui.said", "serve: "+err.Error())
			}
		}()
	}
	wbUI.onOpenPacket = openPacket
	sm.SetUI(wbUI)
	// The tile cache the old workbench already filled: 37 MB of it on this
	// machine, and the same store, so nothing is downloaded twice. The layer
	// is the remembered choice, like the old workbench's.
	if cache, err := os.UserCacheDir(); err == nil {
		layerID := "carto-dark"
		if id := sm.Basemap(); id != "" {
			layerID = id
		}
		mv.Tiles = comp.NewTiles(filepath.Join(cache, "meshcoresim", "tiles"), layerID)
	}
	// The basemap picker at the top of the map's layer panel: base layers
	// only - overlays are not a map - chosen through the shell's chooser and
	// remembered across launches.
	mv.OnBasemap = func() {
		var names []string
		for _, l := range basemap.Layers() {
			if l.Kind == basemap.Base {
				names = append(names, l.Name)
			}
		}
		chooser("Basemap", names, func(picked string) {
			for _, l := range basemap.Layers() {
				if l.Name == picked && mv.Tiles != nil {
					mv.Tiles.SetLayer(l.ID)
					do("map.basemap", map[string]any{"id": l.ID})
				}
			}
		})
	}
	// The map decides, the store changes. A pointer gesture is not allowed to
	// write to the world directly, so both of these go through the same verbs
	// a script would use.
	mv.OnSelect = func(names []string, additive bool) {
		verb := "nodes.select_many"
		if additive {
			verb = "nodes.add_to_selection"
		}
		go func() {
			_, _ = st.Do(ctx, verb, names)
			// The budget follows the selection: it is a panel about whatever
			// is selected, and asking for it separately would let the two
			// disagree about what that is.
			_, _ = st.Do(ctx, "budget.for_selection", nil)
		}()
	}
	mv.OnMove = func(name string, lat, lon float64) {
		go func() {
			_, _ = st.Do(ctx, "nodes.move",
				map[string]any{"name": name, "lat": lat, "lon": lon})
		}()
	}

	if *energyFlag {
		// After the scoreboard has a duty cycle to size the site against.
		go func() {
			time.Sleep(4 * time.Second)
			_, _ = st.Do(ctx, "energy.for_selection", nil)
		}()
	}
	if *saveRunFlag != "" {
		go func() {
			// After the run has had time to produce counters worth recording.
			time.Sleep(18 * time.Second)
			_, _ = st.Do(ctx, "run.save", *saveRunFlag)
		}()
	}
	if *importFlag != "" {
		go func() {
			time.Sleep(9 * time.Second)
			_, _ = st.Do(ctx, "feed.pull", *importFlag)
		}()
	}
	if *importFlag != "" {
		go func() {
			time.Sleep(4 * time.Second)
			_, _ = st.Do(ctx, "import.describe", *importFlag)
		}()
	}
	if *planFlag != "" {
		go func() {
			time.Sleep(6 * time.Second)
			_, _ = st.Do(ctx, "nodes.add_to_selection", []string{*planFlag})
			_, _ = st.Do(ctx, "plan.routes", nil)
		}()
	}
	if *sweepFlag {
		go func() {
			// Once the network is loaded; the sweep builds its own engines.
			time.Sleep(6 * time.Second)
			_, _ = st.Do(ctx, "sweep.run", nil)
		}()
	}
	if *captureFlag != "" {
		// Capture the next transmission rather than this instant.
		//
		// A LoRa packet is tens of milliseconds and the channel is idle
		// between them, so a capture taken at an arbitrary moment is almost
		// always a picture of noise - which is what the first attempt at this
		// produced, correctly and uselessly. Retry until the channel has
		// something on it.
		go func() {
			for i := 0; i < 200; i++ {
				time.Sleep(100 * time.Millisecond)
				_, _ = st.Do(ctx, "waterfall.capture", *captureFlag)
				if snap := st.Snapshot(); snap != nil && snap.Waterfall != nil {
					return
				}
			}
		}()
	}
	if *coverFlag != "" {
		mv.Layers.Coverage = true
		go func() { _, _ = st.Do(ctx, "coverage.compute", *coverFlag) }()
	}
	// A layer that only turns on by a click is a layer nobody can capture.
	for _, l := range strings.Split(*layersFlag, ",") {
		switch strings.TrimSpace(strings.ToLower(l)) {
		case "regions":
			mv.Layers.Regions = true
		case "links":
			mv.Layers.Links = true
		case "coverage":
			mv.Layers.Coverage = true
		case "terrain":
			mv.Layers.Terrain = true
		case "antenna":
			mv.Layers.Pattern = true
		}
	}
	// The map reports what it can see; whatever wants to compute something
	// for that view reads it here rather than duplicating the projection.
	// Guarded: the frame loop writes it and the capture flag's goroutine
	// reads it.
	var viewMu sync.Mutex
	var view [4]float64
	getView := func() [4]float64 {
		viewMu.Lock()
		defer viewMu.Unlock()
		return view
	}
	var lastSize image.Point
	mv.ViewBox = func(south, north, west, east float64) {
		viewMu.Lock()
		view = [4]float64{south, north, west, east}
		viewMu.Unlock()
	}
	mv.OnSize = func(sz image.Point) { lastSize = sz }
	if *shadeFlag {
		mv.Layers.Terrain = true
		go func() {
			// Until the map has drawn once, the view box is the zero value
			// and the verb rightly refuses it. A fixed two-second guess was
			// routinely too early, which is why this flag produced a ticked
			// layer over an unshaded map.
			for i := 0; i < 30; i++ {
				time.Sleep(2 * time.Second)
				if v := getView(); v != [4]float64{} {
					_, _ = st.Do(ctx, "terrain.shade", v)
					return
				}
			}
		}()
	}
	// A menu entry is a name, not a closure: the camera actions are the map's
	// own business, and everything else is a verb the store already has.
	mv.OnMenu = func(action, node string, lat, lon float64) {
		switch action {
		case "map.fit":
			mv.Fit(st.Snapshot(), lastSize)
		case "map.centre":
			mv.FocusOn(st.Snapshot(), node)
		case "map.centre_here":
			mv.CentreOn(lat, lon)
		case "map.neighbours":
			go func() { _, _ = st.Do(ctx, "nodes.select_many", []string{node}) }()
		case "coverage.compute":
			mv.Layers.Coverage = true
			go func() { _, _ = st.Do(ctx, "coverage.compute", node) }()
		case "coverage.clear":
			mv.Layers.Coverage = false
			go func() { _, _ = st.Do(ctx, "coverage.clear", nil) }()
		case "sim.inject":
			go func() { _, _ = st.Do(ctx, "sim.inject", node) }()
		case "panel.pop_out":
			// The shell owns what a pop-out means; the map only says which
			// panel the request was about.
			if sh.OnPopOut != nil {
				sh.OnPopOut("Map")
			}
		default:
			// Everything else is a verb about this node.
			//
			// There was no default, so a menu entry whose action was not in
			// the list above fell through and did nothing at all - which is
			// how "open in its own window" came to be silent after its action
			// was changed. A menu item that reaches nothing must be
			// impossible to write, not merely unlikely.
			go func() {
				if _, err := st.Do(ctx, action, node); err != nil {
					_, _ = st.Do(ctx, "ui.said", action+": "+err.Error())
				}
			}()
		}
	}
	mv.OnLayerOn = func(layer string) {
		switch layer {
		case "Coverage":
			// Coverage is coverage from somewhere, so it needs a node. With
			// none selected the verb refused into a status line and the layer
			// sat ticked over an unchanged map, which is what "coverage does
			// not work" looked like.
			go func() {
				if _, err := st.Do(ctx, "coverage.compute", nil); err != nil {
					_, _ = st.Do(ctx, "ui.said",
						"coverage is coverage from a node: select one, then turn this on")
					return
				}
				_, _ = st.Do(ctx, "ui.said", "working out what it reaches")
			}()
		case "Terrain":
			box := getView()
			go func() { _, _ = st.Do(ctx, "terrain.shade", box) }()
		}
	}

	// Selecting the first node at load also selects its neighbours overlay,
	// so the map opens saying something rather than nothing.
	nodes := &nodesPanel{}
	nodes.OnSelect = func(name string) {
		go func() { _, _ = st.Do(ctx, "nodes.select", name) }()
	}
	mapTop := &mapTools{mv: mv}
	// A double-click on a node is "show me this one".
	mv.OnNodeOpen = func(name string) {
		go func() { _, _ = st.Do(ctx, "node.window", name) }()
	}
	// The place tool puts a node where it was clicked.
	//
	// The kind comes from the toolbar rather than from the map: what a place
	// tool places is a decision about the network, and the map's business is
	// where. Named from the kind and a count, because a node with no name is
	// a node no command can be aimed at.
	mv.OnPlace = func(lat, lon float64) {
		kind, name := mapTop.placeKind, ""
		if kind == "" {
			kind = "simple-repeater"
		}
		if s := st.Snapshot(); s != nil {
			name = nextPlacedName(kind, s)
		}
		go func() {
			if _, err := st.Do(ctx, "nodes.place", map[string]any{
				"name": name, "kind": kind, "lat": lat, "lon": lon,
			}); err != nil {
				_, _ = st.Do(ctx, "ui.said", "place: "+err.Error())
				return
			}
			_, _ = st.Do(ctx, "ui.said", "placed "+name+" - drag it with the move tool")
		}()
	}
	// The link tool asks the question the Inspector asks: what does this link
	// cost, in both directions.
	mv.OnLinkPair = func(a, b string) {
		go func() {
			if _, err := st.Do(ctx, "nodes.select_many", []string{a, b}); err != nil {
				return
			}
			if _, err := st.Do(ctx, "budget.for_selection", nil); err != nil {
				_, _ = st.Do(ctx, "ui.said", "link: "+err.Error())
				return
			}
			_, _ = st.Do(ctx, "ui.said", a+" to "+b+": the budget is in the Link panel")
		}()
	}
	sh.Add(&shell.Panel{Name: "Map", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			wbUI.applyCamera()
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return mapTop.Draw(t, gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return mv.Layout(t, gtx, s)
				}),
			)
		}})
	sh.Add(&shell.Panel{Name: "Nodes", Windowable: true, Draw: nodes.Draw})
	// The Inspector is the events panel's light form, scoped to the
	// selection: what this node has said and heard, not a form about it.
	insp := &eventsPanel{compact: true, forNode: true, OnOpenPacket: openPacket}
	sh.Add(&shell.Panel{Name: "Inspector", Windowable: true, Draw: insp.Draw})
	pkt := &packetPanel{do: do}
	sh.Add(&shell.Panel{Name: "Packet", Windowable: true, Draw: pkt.Draw})
	events := &eventsPanel{OnOpenPacket: openPacket}
	scores := &scorePanel{}
	sh.Add(&shell.Panel{Name: "Events", Windowable: true, Draw: events.Draw})
	sh.Add(&shell.Panel{Name: "Scoreboard", Windowable: true, Draw: scores.Draw})
	tl := &comp.Timeline{}
	sh.Add(&shell.Panel{Name: "Packet timeline", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return tl.Layout(t, gtx, s)
		}})
	sh.Add(&shell.Panel{Name: "Budget", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return comp.Budget{}.Layout(t, gtx, s)
		}})
	sh.Add(&shell.Panel{Name: "Matrix", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return comp.Matrix{}.Layout(t, gtx, s)
		}})
	sh.Add(&shell.Panel{Name: "Energy", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return comp.Energy{}.Layout(t, gtx, s)
		}})
	wf := &comp.Waterfall{}
	sh.Add(&shell.Panel{Name: "Waterfall", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return wf.Layout(t, gtx, s)
		}})
	feed := &feedPanel{}
	feed.OnPull = func() {
		go func() { _, _ = st.Do(ctx, "feed.pull", *importFlag) }()
	}
	sh.Add(&shell.Panel{Name: "Live feed", Windowable: true,
		Draw: withControls(feedCtl.Draw, feed.Draw)})
	sh.Add(&shell.Panel{Name: "Validate", Windowable: true,
		Draw: withControls(validCtl.Draw, (&validatePanel{}).Draw)})
	imp := &importPanel{}
	imp.OnFetch = func(url string) {
		go func() { _, _ = st.Do(ctx, "import.describe", url) }()
	}
	sh.Add(&shell.Panel{Name: "Import", Windowable: true,
		Draw: withControls(importCtl.Draw, imp.Draw)})
	plan := &planPanel{}
	plan.OnRun = func() {
		go func() { _, _ = st.Do(ctx, "plan.routes", nil) }()
	}
	sh.Add(&shell.Panel{Name: "Planning", Windowable: true,
		Draw: withControls(planCtl.Draw, plan.Draw)})
	cmpP := &comparePanel{}
	cmpP.OnSave = func() {
		go func() { _, _ = st.Do(ctx, "run.save", "run") }()
	}
	sh.Add(&shell.Panel{Name: "Compare", Windowable: true, Draw: cmpP.Draw})
	cfg := &configPanel{do: do, choose: chooser}
	if *cfgSection != "" {
		cfg.Open(*cfgSection)
	}
	logp := &logPanel{}
	sh.Add(&shell.Panel{Name: "Provisioning", Windowable: true,
		Draw: withControls(provCtl.Draw, shell.EmptyPanel("Provisioning",
			"what every node is told when it starts").Draw)})
	sh.Add(&shell.Panel{Name: "Configuration", Windowable: true, Draw: cfg.Draw})
	sh.Add(&shell.Panel{Name: "Experiment log", Windowable: true, Draw: logp.Draw})
	lic := &licPanel{}
	sh.Add(&shell.Panel{Name: "Licences", Windowable: true, Draw: lic.Draw})
	nv := &nodeViewPanel{}
	nv.OnAction = func(action, node string) {
		go func() {
			if action == "nodes.stats" {
				_, _ = st.Do(ctx, action, nil)
				return
			}
			_, _ = st.Do(ctx, action, node)
		}()
	}
	nv.OnFirmware = func(node, version string) {
		go func() {
			_, _ = st.Do(ctx, "node.set_firmware",
				map[string]any{"node": node, "version": version})
		}()
	}
	if *filterFlag != "" {
		nv.SetFilter(*filterFlag)
	}
	openOnTab = nodeTab(*nodeTabFlag)
	if *nodeWinFlag != "" {
		go func() {
			time.Sleep(4 * time.Second)
			if _, err := st.Do(ctx, "node.window", *nodeWinFlag); err != nil {
				fmt.Fprintln(os.Stderr, "node.window:", err)
			}
		}()
	}
	if *provFlag != "" {
		go func() {
			if _, err := st.Do(ctx, "node.provisioning", *provFlag); err != nil {
				fmt.Fprintln(os.Stderr, "provisioning:", err)
			}
		}()
	}
	if *openFwFlag != "" {
		nv.OpenFirmware(*openFwFlag)
	}
	if *openMenuFlag != "" {
		go func() {
			// After the first snapshot, so the menu knows whether the
			// node is running and can offer stop rather than start.
			time.Sleep(30 * time.Second)
			nv.OpenMenu(*openMenuFlag, st.Snapshot())
		}()
	}
	sh.Add(&shell.Panel{Name: "Nodes running", Windowable: true, Draw: nv.Draw})
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
			case <-ctx.Done():
				return
			case <-t.C:
				if nv.watched.Swap(false) {
					_, _ = st.Do(ctx, "nodes.stats", nil)
				}
			}
		}
	}()
	fleet := &fleetPanel{}
	bounds := &boundaryPanel{}
	tls := &timelinesPanel{}
	sh.Add(&shell.Panel{Name: "Fleet", Windowable: true,
		Draw: withControls(fleetCtl.Draw, fleet.Draw)})
	sh.Add(&shell.Panel{Name: "Boundary", Windowable: true,
		Draw: withControls(boundCtl.Draw, bounds.Draw)})
	sh.Add(&shell.Panel{Name: "Timelines", Windowable: true, Draw: tls.Draw})
	bench := &benchPanel{}
	bench.OnAction = func(action, node string) {
		go func() {
			switch action {
			case "serve.tcp":
				_, _ = st.Do(ctx, "bench.serve",
					map[string]any{"node": node, "kind": "tcp"})
			case "serve.serial":
				_, _ = st.Do(ctx, "bench.serve",
					map[string]any{"node": node, "kind": "serial"})
			default:
				_, _ = st.Do(ctx, action, nil)
			}
		}()
	}
	sh.Add(&shell.Panel{Name: "Companion bench", Windowable: true,
		Draw: withControls(benchCtl.Draw, bench.Draw)})
	sched := &schedulePanel{}
	console := &consolePanel{}
	sh.Add(&shell.Panel{Name: "Schedule", Windowable: true,
		Draw: withControls(schedCtl.Draw, sched.Draw)})
	sh.Add(&shell.Panel{Name: "Link", Windowable: true, Draw: linkPanel{}.Draw})
	sh.Add(&shell.Panel{Name: "Console", Windowable: true, Draw: console.Draw})
	fw := &firmwarePanel{choose: chooser}
	// The library asks for itself, and asks again after anything that changes
	// it. A panel that reads the cache directly cannot know when a download
	// has landed.
	fw.Refresh = func() {
		go func() { _, _ = st.Do(ctx, "firmware.library", nil) }()
	}
	fw.OnAction = func(verb string, params map[string]any) {
		go func() {
			if _, err := st.Do(ctx, verb, params); err != nil {
				_, _ = st.Do(ctx, "ui.said", verb+": "+err.Error())
			}
			_, _ = st.Do(ctx, "firmware.library", nil)
		}()
	}
	// Importing needs a path and a role, which a button cannot carry: ask for
	// both through the shell, then refresh so the new build appears.
	fw.OnImport = func() {
		sh.Ask.Open("Import a build from", "path to a binary", "", func(path string) {
			if strings.TrimSpace(path) == "" {
				return
			}
			sh.Ask.Post(func(ask *shell.Prompt) {
				ask.Choose("Import it as which role?", "filter", []string{
					"simple_repeater", "advanced_repeater", "companion_radio",
					"simple_room_server",
				}, func(role string) {
					go func() {
						if _, err := st.Do(ctx, "firmware.import",
							map[string]any{"path": path, "role": role}); err != nil {
							_, _ = st.Do(ctx, "ui.said", "import: "+err.Error())
						}
						_, _ = st.Do(ctx, "firmware.library", nil)
					}()
				})
			})
		})
	}
	runs := &runsPanel{}
	sh.Add(&shell.Panel{Name: "Firmware", Windowable: true, Draw: fw.Draw})
	sh.Add(&shell.Panel{Name: "Runs", Windowable: true, Draw: runs.Draw})
	sh.Add(&shell.Panel{Name: "Sweep", Windowable: true,
		Draw: withControls(sweepCtl.Draw, (&sweepResults{}).Draw)})
	switch *viewFlag {
	case "run":
		sh.View = shell.Run
	case "debug":
		sh.View = shell.Debug
	case "verify":
		sh.View = shell.Verify
	case "bench":
		sh.View = shell.Bench
	case "app":
		sh.View = shell.App
	}

	sh2 := text.NewShaper(text.WithCollection(withEmoji(gofont.Collection())))
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

	// The interface's own settings live on the Configuration page now, under
	// Interface - a Settings panel beside a Configuration page was two homes
	// for one kind of thing.
	cfg.sets = sets
	// File is where a session begins and ends, and it had one item in it.
	//
	// Firmware lives here rather than under Repeaters because a companion, a
	// room server and an SDR observer all run firmware too - filing it under
	// one node type is how somebody looking for a companion build never finds
	// it.
	for _, m := range workbenchMenus() {
		sh.SetMenu(m.Name, m.Items)
	}
	// The Window menu had become a dumping ground: every panel, alphabetical,
	// thirty rows deep. It lists the daily set; everything else stays one
	// step away behind "Show all panels...".
	for _, name := range []string{
		"Map", "Events", "Nodes", "Nodes running", "Console", "Fleet",
		"Firmware", "Configuration", "Packet timeline", "Packet",
		"Waterfall", "Companion bench", "Experiment log",
	} {
		if p := sh.Panels[name]; p != nil {
			p.InWindowMenu = true
		}
	}
	sh.WindowMenu("Window")
	if *dropFlag != "" {
		// Before the frame loop starts, so no goroutine races the renderer.
		sh.OpenMenu(*dropFlag)
	}
	sh.OnMenu = func(action string) {
		if name, ok := strings.CutPrefix(action, "panel."); ok {
			sh.OnPopOut(name)
			return
		}
		// The Window menu's overflow: every panel in the chooser, for when
		// the scrolling list is more list than anybody wants to scroll.
		if action == "window.showall" {
			var names []string
			for n, p := range sh.Panels {
				if p != nil && p.Windowable {
					names = append(names, n)
				}
			}
			sort.Strings(names)
			sh.Ask.Post(func(ask *shell.Prompt) {
				ask.Choose("Open a panel", "filter", names, func(name string) {
					sh.OnPopOut(name)
				})
			})
			return
		}
		// Opening one of your own networks: the names are already known, so the
		// question offers them rather than asking for a path. Before this the
		// entry opened the live-import panel, which is a different thing that
		// happens to also produce nodes.
		if action == "project.open" {
			go func() {
				res, err := st.Do(ctx, "project.list", nil)
				if err != nil {
					return
				}
				m, _ := res.(map[string]any)
				dir, _ := m["dir"].(string)
				var names []string
				if raw, ok := m["projects"].([]string); ok {
					names = raw
				}
				sh.Ask.Post(func(ask *shell.Prompt) {
					ask.Choose("Open a saved network", "filter", names,
						func(name string) {
							go func() {
								_, _ = st.Do(ctx, "project.open",
									filepath.Join(dir, name+".json"))
							}()
						})
				})
			}()
			return
		}
		// Play, when the nodes have no build to run.
		//
		// The verb refuses, correctly - a run with half a mesh up measures a
		// network that does not exist. But the refusal named 34 nodes and left
		// the operator to go and pin them, which is not a thing anybody does one
		// node at a time. Ask by role instead, which is how firmware is chosen
		// anyway: one answer covers every repeater.
		if action == "sim.start" {
			go func() {
				res, err := st.Do(ctx, "firmware.needed", nil)
				if err == nil {
					if roles := rolesNeeding(res); len(roles) > 0 {
						askForFirmware(ctx, st, &sh.Ask, roles)
						return
					}
				}
				_, _ = st.Do(ctx, "sim.start", nil)
			}()
			return
		}
		// Planning between two nodes, when two are not already selected.
		//
		// The verb reads the selection, so from a menu it simply refused, and
		// "select two nodes to plan between" in a status bar is a rebuke rather
		// than an instruction. Asking for the ends is the instruction.
		if action == "plan.routes" {
			if sel := selectedNames(st.Snapshot()); len(sel) < 2 {
				askForRouteEnds(ctx, st, &sh.Ask, sel)
				return
			}
		}
		// Verbs that need a word from the operator. A menu entry carries no
		// parameters, so before this the item fired, the verb refused, and the
		// only trace was a line in the status bar nobody was reading.
		if ask, ok := menuAsks[action]; ok {
			sh.Ask.Open(ask.title, ask.hint, ask.initial(), func(answer string) {
				if strings.TrimSpace(answer) == "" {
					return
				}
				go func() {
					_, _ = st.Do(ctx, action, map[string]any{ask.field: answer})
				}()
			})
			return
		}
		// The interface's own settings are a Configuration section now.
		if action == "config.interface" {
			cfg.Open("Interface")
			sh.OnPopOut("Configuration")
			return
		}
		if action == "ui.toggle_real_firmware" {
			// Read the current value and send its opposite, so the control
			// never has its own copy of the answer to drift from the store's.
			go func() {
				real := false
				if s := st.Snapshot(); s != nil {
					real = s.RealFirmware
				}
				_, _ = st.Do(ctx, "sim.kind", map[string]any{"real": !real})
			}()
			return
		}
		go func() { _, _ = st.Do(ctx, action, nil) }()
	}
	if *menuFlag != "" {
		sh.OnMenu(*menuFlag)
	}
	wbUI.newTheme = func() *theme.Theme {
		m, d, _ := sets.get()
		return theme.New(m, d,
			text.NewShaper(text.WithCollection(withEmoji(gofont.Collection()))))
	}
	wbUI.dock = func(name string) { wins.dock(name) }
	wbUI.closeWin = func(name string) error { return wins.close(name) }
	wbUI.scale = sets.getScale
	wbUI.setScale = sets.setScale
	sh.PoppedOut = wins.has
	sh.OnPopOut = func(name string) {
		wins.popOut(name, sh, func() *theme.Theme {
			// A shaper of its own, per window, and the current settings so a
			// window opened after a theme change does not open in the old one.
			m, d, _ := sets.get()
			return theme.New(m, d,
				text.NewShaper(text.WithCollection(withEmoji(gofont.Collection()))))
		}, st)
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

func withEmoji(base []font.FontFace) []font.FontFace {
	// The bundle carries the font beside the binary, so emoji in node names
	// do not depend on what the machine happens to have installed.
	paths := []string{
		"/usr/share/fonts/noto/NotoColorEmoji.ttf",
		"/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf",
		"/System/Library/Fonts/Apple Color Emoji.ttc",
	}
	if exe, err := os.Executable(); err == nil {
		paths = append([]string{
			filepath.Join(filepath.Dir(exe), "fonts", "NotoColorEmoji.ttf"),
		}, paths...)
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if faces, err := opentype.ParseCollection(b); err == nil {
			return append(base, faces...)
		}
	}
	return base
}

func shortKind(k string) string {
	switch k {
	case "simple-repeater":
		return "repeater"
	case "advanced-repeater":
		return "advanced"
	case "sdr-observer":
		return "observer"
	case "room-server":
		return "room server"
	}
	return k
}

func kindOf(k string) theme.NodeKind {
	switch k {
	case "companion":
		return theme.Companion
	case "room-server":
		return theme.RoomServer
	case "sdr-observer":
		return theme.Observer
	case "emitter":
		return theme.Emitter
	case "advanced-repeater":
		return theme.AdvancedRepeater
	}
	return theme.SimpleRepeater
}

func itoa(i int) string { return fmt.Sprintf("%d", i) }
func ftoa(f float64) string {
	if f == float64(int(f)) {
		return fmt.Sprintf("%.0f", f)
	}
	return fmt.Sprintf("%.5f", f)
}
func join(v []string) string {
	out := ""
	for i, s := range v {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}

var _ = widget.Clickable{}
var _ = time.Now

// menuAsks maps a verb to the one thing it needs before it can run.
//
// Keeping it beside the menu rather than inside the verb is deliberate: the
// verb is also driven by scripts, which supply the parameter directly and
// should not be made to answer a question.
var menuAsks = map[string]struct {
	title, hint, field string
	initial            func() string
}{
	"project.save": {
		title: "Save this network as",
		hint:  "a name, no extension",
		field: "name",
		initial: func() string {
			return "network-" + time.Now().Format("20060102-1504")
		},
	},
}

// selectedNames is who is selected right now.
func selectedNames(s *state.Snapshot) []string {
	if s == nil {
		return nil
	}
	var out []string
	for i := range s.Nodes {
		if s.Nodes[i].Selected {
			out = append(out, s.Nodes[i].Name)
		}
	}
	return out
}

// askForRouteEnds gets the two ends the route search needs, then selects them
// and runs it - so the selection ends up saying what was planned, which is
// what somebody who selected the nodes by hand would have seen.
func askForRouteEnds(ctx context.Context, st *state.Store, ask *shell.Prompt,
	sel []string) {
	s := st.Snapshot()
	if s == nil || len(s.Nodes) < 2 {
		return
	}
	names := make([]string, 0, len(s.Nodes))
	for i := range s.Nodes {
		names = append(names, s.Nodes[i].Name)
	}
	run := func(from, to string) {
		go func() {
			_, _ = st.Do(ctx, "nodes.select_many", []string{from, to})
			_, _ = st.Do(ctx, "plan.routes", nil)
		}()
	}
	if len(sel) == 1 {
		from := sel[0]
		ask.Choose("Plan a route from "+from+" to", "filter",
			except(names, from), func(to string) { run(from, to) })
		return
	}
	ask.Choose("Plan a route from", "filter", names, func(from string) {
		ask.Choose("Plan a route from "+from+" to", "filter",
			except(names, from), func(to string) { run(from, to) })
	})
}

func except(names []string, drop string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n != drop {
			out = append(out, n)
		}
	}
	return out
}

// roleNeed is one role with nodes that have nothing to run.
type roleNeed struct {
	role    string
	nodes   int
	choices []string
}

// rolesNeeding reads firmware.needed's answer.
func rolesNeeding(res any) []roleNeed {
	m, _ := res.(map[string]any)
	raw, _ := m["roles"].([]any)
	var out []roleNeed
	for _, r := range raw {
		rm, _ := r.(map[string]any)
		n := roleNeed{role: fmt.Sprint(rm["role"])}
		switch v := rm["nodes"].(type) {
		case int:
			n.nodes = v
		case float64:
			n.nodes = int(v)
		}
		if cs, ok := rm["choices"].([]string); ok {
			n.choices = cs
		}
		out = append(out, n)
	}
	return out
}

// askForFirmware asks what each role should run, then starts the run.
//
// One question per role, in turn, because two modal questions at once is a
// state nobody can reason about. A role with nothing installed is said aloud
// rather than skipped silently - "the run did not start" and "there is no
// companion build on this machine" are not the same problem.
func askForFirmware(ctx context.Context, st *state.Store, ask *shell.Prompt,
	roles []roleNeed) {
	if len(roles) == 0 {
		go func() { _, _ = st.Do(ctx, "sim.start", nil) }()
		return
	}
	r := roles[0]
	rest := roles[1:]
	if len(r.choices) == 0 {
		go func() {
			_, _ = st.Do(ctx, "ui.said", fmt.Sprintf(
				"%d %s have nothing to run, and no build for them is installed - "+
					"download one in the Firmware library",
				r.nodes, readableRole(r.role)))
		}()
		return
	}
	title := fmt.Sprintf("What should the %d %s run?", r.nodes, readableRole(r.role))
	ask.Post(func(a *shell.Prompt) {
		a.Choose(title, "filter", r.choices, func(version string) {
			go func() {
				_, _ = st.Do(ctx, "firmware.set",
					map[string]any{"role": r.role, "version": version})
				askForFirmware(ctx, st, a, rest)
			}()
		})
	})
}

// readableRole says a role the way somebody would.
//
// The token is what firmware.set matches on and is not for reading:
// "simple_room_server" in a question about what to run is the program
// talking to itself.
func readableRole(role string) string {
	switch role {
	case "simple_repeater":
		return "repeaters"
	case "advanced_repeater":
		return "advanced repeaters"
	case "companion_radio":
		return "companions"
	case "simple_room_server":
		return "room servers"
	}
	return strings.ReplaceAll(role, "_", " ") + " nodes"
}

// menu is one menu bar heading and what is under it.
type menu struct {
	Name  string
	Items []shell.MenuItem
}

// workbenchMenus is the menu bar, in one place.
//
// It used to be a run of SetMenu calls, with the test that presses every entry
// keeping its own copy. The copy drifted - it was still pressing the entry that
// opened the import panel after that entry had become "open a saved network" -
// so the test passed while checking a menu bar nobody had.
// The sections, icons and shortcuts are the UX design's, verbatim: grouped by
// purpose under headings, the most common action first in each menu, and the
// binding shown in the row is the binding the shell registers - one table
// feeds both, so they cannot drift apart.
func workbenchMenus() []menu {
	return []menu{
		{"File", []shell.MenuItem{
			{Label: "Open a saved network", Action: "project.open",
				Section: "Open & Save", Icon: "folder", Shortcut: "Ctrl+O"},
			{Label: "Save this network", Action: "project.save",
				Section: "Open & Save", Icon: "save", Shortcut: "Ctrl+S"},
			{Label: "Save this run", Action: "run.save",
				Section: "Open & Save", Icon: "save", Shortcut: "Ctrl+Shift+S"},
			{Label: "Firmware library", Action: "panel.Firmware",
				Section: "Import & Export", Icon: "chip"},
			{Label: "Import a live network", Action: "panel.Import",
				Section: "Import & Export", Icon: "import"},
			{Label: "Export the event log", Action: "events.dump",
				Section: "Import & Export", Icon: "export"},
			{Label: "Quit", Action: "app.quit",
				Section: "Exit", Icon: "exit", Shortcut: "Ctrl+Q"},
		}},
		{"View", []shell.MenuItem{
			{Label: "Nodes running", Action: "panel.Nodes running",
				Section: "Overview", Icon: "nodes"},
			{Label: "Companion bench", Action: "panel.Companion bench",
				Section: "Overview", Icon: "chip"},
			{Label: "Experiment log", Action: "panel.Experiment log",
				Section: "Diagnostics", Icon: "log"},
			{Label: "Configuration", Action: "panel.Configuration",
				Section: "Diagnostics", Icon: "sliders"},
			{Label: "Settings", Action: "config.interface",
				Section: "Preferences", Icon: "settings"},
		}},
		{"Simulation", []shell.MenuItem{
			{Label: "Play or pause", Action: "sim.start",
				Section: "Control", Icon: "play", Shortcut: "Space"},
			{Label: "One step", Action: "sim.step",
				Section: "Control", Icon: "step"},
			{Label: "Back to the start", Action: "sim.reset",
				Section: "Control", Icon: "back"},
			{Label: "Start firmware on every node", Action: "firmware.start",
				Section: "Nodes", Icon: "power"},
			{Label: "Wipe every node's memory", Action: "firmware.wipe",
				Section: "Nodes", Icon: "wipe"},
			{Label: "Originate a packet", Action: "sim.inject",
				Section: "Tools", Icon: "packet"},
			{Label: "Capture the waterfall", Action: "waterfall.capture",
				Section: "Tools", Icon: "waterfall"},
			{Label: "Capture to a pcapng file", Action: "capture.file",
				Section: "Tools", Icon: "pcap"},
		}},
		{"Repeaters", []shell.MenuItem{
			{Label: "Send a command to the fleet", Action: "panel.Fleet",
				Section: "Commands", Icon: "send"},
			{Label: "What they are told at boot", Action: "panel.Provisioning",
				Section: "Boot", Icon: "boot"},
			{Label: "Coverage from the selection", Action: "coverage.compute",
				Section: "Analysis", Icon: "coverage"},
		}},
		// Import is under File, with opening and saving, and not here as
		// well: one entry in two menus is two entries to keep in step and one
		// of them is always the stale one.
		{"Planning", []shell.MenuItem{
			{Label: "Routes between two selected nodes", Action: "plan.routes",
				Section: "Tools", Icon: "route"},
			{Label: "Boundary", Action: "panel.Boundary",
				Section: "Tools", Icon: "boundary"},
		}},
		// Window is generated from the panels themselves, so it is not here.
		{"Help", []shell.MenuItem{
			{Label: "What this run assumes", Action: "panel.Configuration",
				Icon: "help"},
			{Label: "Licences & attributions", Action: "panel.Licences",
				Icon: "doc"},
		}},
	}
}

// nextPlacedName is a name nothing else has, in the kind's own words.
func nextPlacedName(kind string, s *state.Snapshot) string {
	base := strings.ReplaceAll(kind, "simple-", "")
	base = strings.ReplaceAll(base, "-", " ")
	taken := map[string]bool{}
	for i := range s.Nodes {
		taken[s.Nodes[i].Name] = true
	}
	for n := 1; ; n++ {
		name := fmt.Sprintf("new %s %d", base, n)
		if !taken[name] {
			return name
		}
	}
}
