// The flags that make the workbench do something the moment it opens.
//
// Every one of them exists so a result can be captured or scripted without a
// hand on the mouse: -coverage draws a raster, -sweep runs the matrix, -plan
// asks for a route. They ran inline in Run and were most of its length once
// the panels moved out.
//
// They fire on workers, never here. Run has not shown a window yet at this
// point, and an application that computes a national coverage raster before
// it appears is indistinguishable from one that has crashed.
package workbench

import (
	"context"
	"image"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/shell"
)

// startupActions is what the startup flags need to act on.
type startupActions struct {
	st  *state.Store
	ctx context.Context
	mv  *comp.MapView
	sh  *shell.Shell

	saveRunFlag, importFlag, planFlag  *string
	captureFlag, coverFlag, layersFlag *string
	lookFlag                           *string
	energyFlag, sweepFlag, shadeFlag   *bool
}

// run fires whichever of them were asked for.
func (a startupActions) run() {
	if *a.energyFlag {
		// After the scoreboard has a duty cycle to size the site against.
		go func() {
			time.Sleep(4 * time.Second)
			_, _ = a.st.Do(a.ctx, "energy.for_selection", nil)
		}()
	}
	if *a.saveRunFlag != "" {
		go func() {
			// After the run has had time to produce counters worth recording.
			time.Sleep(18 * time.Second)
			_, _ = a.st.Do(a.ctx, "run.save", *a.saveRunFlag)
		}()
	}
	if *a.importFlag != "" {
		go func() {
			time.Sleep(9 * time.Second)
			_, _ = a.st.Do(a.ctx, "feed.pull", *a.importFlag)
		}()
	}
	if *a.importFlag != "" {
		go func() {
			time.Sleep(4 * time.Second)
			_, _ = a.st.Do(a.ctx, "import.describe", *a.importFlag)
		}()
	}
	if *a.planFlag != "" {
		go func() {
			time.Sleep(6 * time.Second)
			_, _ = a.st.Do(a.ctx, "nodes.add_to_selection", []string{*a.planFlag})
			_, _ = a.st.Do(a.ctx, "plan.routes", nil)
		}()
	}
	if *a.sweepFlag {
		go func() {
			// Once the network is loaded; the sweep builds its own engines.
			time.Sleep(6 * time.Second)
			_, _ = a.st.Do(a.ctx, "sweep.run", nil)
		}()
	}
	if *a.captureFlag != "" {
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
				_, _ = a.st.Do(a.ctx, "waterfall.capture", *a.captureFlag)
				if snap := a.st.Snapshot(); snap != nil && snap.Waterfall != nil {
					return
				}
			}
		}()
	}
	if *a.lookFlag != "" {
		// lat,lon,zoom: a capture cannot drag the map to the place it is
		// meant to be capturing.
		parts := strings.Split(*a.lookFlag, ",")
		if len(parts) == 3 {
			lat, e1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			lon, e2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			zoom, e3 := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
			if e1 == nil && e2 == nil && e3 == nil {
				a.mv.StartAt(lat, lon, zoom)
			}
		}
	}
	if *a.coverFlag != "" {
		a.mv.Layers.Coverage = true
		go func() { _, _ = a.st.Do(a.ctx, "coverage.compute", *a.coverFlag) }()
	}
	// A layer that only turns on by a click is a layer nobody can capture.
	for _, l := range strings.Split(*a.layersFlag, ",") {
		switch strings.TrimSpace(strings.ToLower(l)) {
		case "regions":
			a.mv.Layers.Regions = true
		case "links":
			a.mv.Layers.Links = true
		case "coverage":
			a.mv.Layers.Coverage = true
		case "terrain":
			a.mv.Layers.Terrain = true
		case "buildings":
			a.mv.Layers.Buildings = true
		case "antenna":
			a.mv.Layers.Pattern = true
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
	a.mv.ViewBox = func(south, north, west, east float64) {
		viewMu.Lock()
		view = [4]float64{south, north, west, east}
		viewMu.Unlock()
	}
	a.mv.OnSize = func(sz image.Point) { lastSize = sz }
	if *a.shadeFlag {
		a.mv.Layers.Terrain = true
		go func() {
			// Until the map has drawn once, the view box is the zero value
			// and the verb rightly refuses it. A fixed two-second guess was
			// routinely too early, which is why this flag produced a ticked
			// layer over an unshaded map.
			for i := 0; i < 30; i++ {
				time.Sleep(2 * time.Second)
				if v := getView(); v != [4]float64{} {
					_, _ = a.st.Do(a.ctx, "terrain.shade", v)
					return
				}
			}
		}()
	}
	// A menu entry is a name, not a closure: the camera actions are the map's
	// own business, and everything else is a verb the store already has.
	a.mv.OnMenu = func(action, node string, lat, lon float64) {
		switch action {
		case "map.fit":
			a.mv.Fit(a.st.Snapshot(), lastSize)
		case "map.centre":
			a.mv.FocusOn(a.st.Snapshot(), node)
		case "map.centre_here":
			a.mv.CentreOn(lat, lon)
		case "link.from":
			a.mv.ArmLink(node, lat, lon)
		case "link.to":
			a.mv.LinkTo(node, lat, lon)
		case "map.neighbours":
			go func() { _, _ = a.st.Do(a.ctx, "nodes.select_many", []string{node}) }()
		case "coverage.compute":
			a.mv.Layers.Coverage = true
			go func() { _, _ = a.st.Do(a.ctx, "coverage.compute", node) }()
		case "coverage.clear":
			a.mv.Layers.Coverage = false
			go func() { _, _ = a.st.Do(a.ctx, "coverage.clear", nil) }()
		case "sim.inject":
			go func() { _, _ = a.st.Do(a.ctx, "sim.inject", node) }()
		case "nodes.delete":
			// Not through the default below, which would call the verb
			// straight away. Deleting is the one entry here that asks first,
			// and it asks in the same place the Delete key does so the two
			// cannot come to behave differently.
			if a.mv.OnDelete != nil {
				a.mv.OnDelete([]string{node})
			}
		case "panel.pop_out":
			// The shell owns what a pop-out means; the map only says which
			// panel the request was about.
			if a.sh.OnPopOut != nil {
				a.sh.OnPopOut("Map")
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
				if _, err := a.st.Do(a.ctx, action, node); err != nil {
					_, _ = a.st.Do(a.ctx, "ui.said", action+": "+err.Error())
				}
			}()
		}
	}
	a.mv.OnLayerOn = func(layer string) {
		switch layer {
		case "Coverage":
			// Coverage is coverage from somewhere, so it needs a node. With
			// none selected the verb refused into a status line and the layer
			// sat ticked over an unchanged map, which is what "coverage does
			// not work" looked like.
			go func() {
				if _, err := a.st.Do(a.ctx, "coverage.compute", nil); err != nil {
					_, _ = a.st.Do(a.ctx, "ui.said",
						"coverage is coverage from a node: select one, then turn this on")
					return
				}
				_, _ = a.st.Do(a.ctx, "ui.said", "working out what it reaches")
			}()
		case "Terrain":
			box := getView()
			go func() { _, _ = a.st.Do(a.ctx, "terrain.shade", box) }()
		}
	}

	// Selecting the first node at load also selects its neighbours overlay,
	// so the map opens saying something rather than nothing.
}
