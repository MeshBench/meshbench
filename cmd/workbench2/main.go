// The new workbench: the shell, the state layer and the panels, wired
// together. Run alongside the old UI until the cutover.
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime/pprof"
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

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/desktop"
	"github.com/A13xB0/meshcoresim/internal/gui/shell"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

func main() {
	fixture := flag.String("fixture", "fixtures/fixture-scotland-ireland-strict.json",
		"network to load")
	modeFlag := flag.String("theme", "dark", "dark or light")
	viewFlag := flag.String("view", "plan", "which view to open")
	fpsFlag := flag.Bool("fps", false, "report frames per second to stderr and /tmp/wb2-fps.log")
	panelFlag := flag.String("panel", "", "draw only this panel, filling the window")
	profFlag := flag.String("cpuprofile", "", "write a CPU profile here")
	playFlag := flag.Bool("play", false, "start the simulation immediately")
	injectFlag := flag.String("inject", "", "originate a packet at this node once running")
	importFlag := flag.String("import", "", "describe an import from this CoreScope URL at startup")
	planFlag := flag.String("plan", "", "plan between the selected node and this one at startup")
	sweepFlag := flag.Bool("sweep", false, "run the default sweep at startup")
	saveRunFlag := flag.String("save-run", "", "save a run record under this name, then keep running")
	shadeFlag := flag.Bool("terrain", false, "shade the relief at startup")
	coverFlag := flag.String("coverage", "",
		"compute and show coverage from this node at startup")
	energyFlag := flag.Bool("energy", false, "run the site study for the selected node at startup")
	captureFlag := flag.String("capture", "",
		"capture the waterfall at this node once the run has traffic")
	injectEvery := flag.Duration("inject-every", 0,
		"keep originating at that node this often; for looking at the traffic layer")
	quitFlag := flag.Duration("quit-after", 0, "exit after this long; 0 runs until closed")
	flag.Parse()

	st := state.New(10)
	sm := &sim{}
	registerVerbs(st, sm)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

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

	sh := shell.New()
	mv := &comp.MapView{}
	// The tile cache the old workbench already filled: 37 MB of it on this
	// machine, and the same store, so nothing is downloaded twice.
	if cache, err := os.UserCacheDir(); err == nil {
		mv.Tiles = comp.NewTiles(filepath.Join(cache, "meshcoresim", "tiles"), "carto-dark")
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
	// The map reports what it can see; whatever wants to compute something
	// for that view reads it here rather than duplicating the projection.
	var view [4]float64
	var lastSize image.Point
	mv.ViewBox = func(south, north, west, east float64) {
		view = [4]float64{south, north, west, east}
	}
	mv.OnSize = func(sz image.Point) { lastSize = sz }
	if *shadeFlag {
		mv.Layers.Terrain = true
		go func() {
			// After a frame or two, so the view box is the view rather than
			// the zero value.
			time.Sleep(2 * time.Second)
			_, _ = st.Do(ctx, "terrain.shade", view)
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
		}
	}
	mv.OnLayerOn = func(layer string) {
		switch layer {
		case "Coverage":
			go func() { _, _ = st.Do(ctx, "coverage.compute", nil) }()
		case "Terrain":
			box := view
			go func() { _, _ = st.Do(ctx, "terrain.shade", box) }()
		}
	}

	// Selecting the first node at load also selects its neighbours overlay,
	// so the map opens saying something rather than nothing.
	nodes := &nodesPanel{}
	sh.Add(&shell.Panel{Name: "Map", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return mv.Layout(t, gtx, s)
		}})
	sh.Add(&shell.Panel{Name: "Nodes", Windowable: true, Draw: nodes.Draw})
	sh.Add(&shell.Panel{Name: "Inspector", Windowable: true, Draw: drawInspector})
	events := &eventsPanel{}
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
	imp := &importPanel{}
	imp.OnFetch = func(url string) {
		go func() { _, _ = st.Do(ctx, "import.describe", url) }()
	}
	sh.Add(&shell.Panel{Name: "Import", Windowable: true, Draw: imp.Draw})
	plan := &planPanel{}
	plan.OnRun = func() {
		go func() { _, _ = st.Do(ctx, "plan.routes", nil) }()
	}
	sh.Add(&shell.Panel{Name: "Planning", Windowable: true, Draw: plan.Draw})
	cmpP := &comparePanel{}
	cmpP.OnSave = func() {
		go func() { _, _ = st.Do(ctx, "run.save", "run") }()
	}
	sh.Add(&shell.Panel{Name: "Compare", Windowable: true, Draw: cmpP.Draw})
	cfg := &configPanel{}
	logp := &logPanel{}
	sh.Add(&shell.Panel{Name: "Configuration", Windowable: true, Draw: cfg.Draw})
	sh.Add(&shell.Panel{Name: "Experiment log", Windowable: true, Draw: logp.Draw})
	fleet := &fleetPanel{}
	bounds := &boundaryPanel{}
	tls := &timelinesPanel{}
	sh.Add(&shell.Panel{Name: "Fleet", Windowable: true, Draw: fleet.Draw})
	sh.Add(&shell.Panel{Name: "Boundary", Windowable: true, Draw: bounds.Draw})
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
	sh.Add(&shell.Panel{Name: "Companion bench", Windowable: true, Draw: bench.Draw})
	sched := &schedulePanel{}
	console := &consolePanel{}
	sh.Add(&shell.Panel{Name: "Schedule", Windowable: true, Draw: sched.Draw})
	sh.Add(&shell.Panel{Name: "Link", Windowable: true, Draw: linkPanel{}.Draw})
	sh.Add(&shell.Panel{Name: "Console", Windowable: true, Draw: console.Draw})
	fw := &firmwarePanel{}
	runs := &runsPanel{}
	sh.Add(&shell.Panel{Name: "Firmware", Windowable: true, Draw: fw.Draw})
	sh.Add(&shell.Panel{Name: "Runs", Windowable: true, Draw: runs.Draw})
	for _, p := range []struct{ name, what string }{
		{"Validate", "residuals against reality - P6"},
		{"Sweep", "arms, seeds, senders - P6"},
	} {
		sh.Add(shell.EmptyPanel(p.name, p.what))
	}
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
	th := theme.New(mode, theme.Default, sh2)

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
			os.Exit(0)
		}()
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
				if p := sh.Panels[*panelFlag]; p != nil {
					comp.Fill(gtx, th.P.Ground)
					p.Draw(th, gtx, st.Snapshot())
				} else {
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
	for _, p := range []string{
		"/usr/share/fonts/noto/NotoColorEmoji.ttf",
		"/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf",
		"/System/Library/Fonts/Apple Color Emoji.ttc",
	} {
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
