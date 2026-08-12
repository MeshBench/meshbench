// The new workbench: the shell, the state layer and the panels, wired
// together. Run alongside the old UI until the cutover.
package main

import (
	"context"
	"flag"
	"fmt"
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
		go func() { _, _ = st.Do(ctx, verb, names) }()
	}
	mv.OnMove = func(name string, lat, lon float64) {
		go func() {
			_, _ = st.Do(ctx, "nodes.move",
				map[string]any{"name": name, "lat": lat, "lon": lon})
		}()
	}

	nodes := &nodesPanel{}
	sh.Add(&shell.Panel{Name: "Map", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return mv.Layout(t, gtx, s)
		}})
	sh.Add(&shell.Panel{Name: "Nodes", Windowable: true, Draw: nodes.Draw})
	sh.Add(&shell.Panel{Name: "Inspector", Windowable: true, Draw: drawInspector})
	for _, p := range []struct{ name, what string }{
		{"Schedule", "sends and assertions - P6"},
		{"Scoreboard", "per-node counters - P4"},
		{"Packet timeline", "lanes on a shared time axis - P5"},
		{"Link", "both directions, always - P6"},
		{"Compare", "two runs, metric by metric - P6"},
		{"Validate", "residuals against reality - P6"},
		{"Runs", "every run with its build checksum - P4"},
		{"Sweep", "arms, seeds, senders - P6"},
		{"Matrix", "arms against seeds - P5"},
		{"Companion bench", "an endpoint to point your client at - P6"},
		{"Events", "every event with its cause - P4"},
		{"Console", "per-node tabs - P6"},
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
