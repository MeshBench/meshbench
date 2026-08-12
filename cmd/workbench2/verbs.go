// Control verbs, registered on the store.
//
// The parity test in 12.9 is generated from what is registered here rather
// than from a list in a document, which was already wrong by three verbs.
package main

import (
	"context"
	"fmt"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

// registerVerbs wires the control verbs onto the store. Only the few the new
// UI needs so far; the rest arrive as their panels do, and the parity test in
// 12.9 is generated from what is registered here.
func registerVerbs(st *state.Store, s *sim) {
	st.Handle("project.open", func(w *state.World, p any) (any, error) {
		path, _ := p.(string)
		f, err := loadFixture(path)
		if err != nil {
			return nil, err
		}
		w.Nodes, w.Areas, w.MarginKm = f.nodes, f.areas, f.margin
		w.Seed = 9001

		// Build the engine, but do not ask it for margins here.
		//
		// A margin is a path loss, and a path loss over real terrain is a
		// profile sampled along the ground. 48,000 of them is minutes, and
		// this handler runs on the store's goroutine with the window not yet
		// open - which is exactly how the first attempt produced an
		// application that never appeared. So: a job, and a map that draws
		// proximity links until the real ones arrive.
		s.build(f.scene, 869.618)
		w.Links = nil
		s.warm(st, len(f.scene))
		// One engine step per tick. Step is the engine's own unit of time
		// and takes its size from the config, so the store paces it rather
		// than redefining it.
		// eventTail is how many of the most recent events the tables show. A
		// run of an hour has millions, and a table nobody can scroll to the
		// end of is not more honest than one that says how many there were.
		const eventTail = 2000
		index := map[string]int{}
		for i, n := range f.nodes {
			index[n.Name] = i
		}
		w.Tick = func(uint32) {
			_ = s.eng.Step(context.Background())
			w.NowMs = s.eng.NowMs()
			// Trails from the last few seconds of simulated time. Recomputed
			// from the event log rather than accumulated, so a seek backwards
			// or a rebuilt engine cannot leave a trail on the map for a
			// transmission that is no longer in the run.
			const trailWindowMs = 4000
			from := uint32(0)
			if w.NowMs > trailWindowMs {
				from = w.NowMs - trailWindowMs
			}
			w.Trails = s.trailsSince(from, index)
			w.Events, w.EventTotal = s.eventTail(eventTail)
			w.Scores = s.scores()
		}

		w.Say(fmt.Sprintf("opened %s: %d nodes, %d links, %d areas",
			path, len(f.nodes), len(w.Links), len(f.areas)))
		return map[string]any{
			"opened": path, "nodes": len(f.nodes), "links": len(w.Links),
		}, nil
	})
	st.Handle("sim.play", func(w *state.World, _ any) (any, error) {
		w.Playing = true
		w.Say("playing")
		return map[string]any{"playing": true}, nil
	})
	st.Handle("sim.pause", func(w *state.World, _ any) (any, error) {
		w.Playing = false
		w.Say("paused")
		return map[string]any{"playing": false}, nil
	})
	st.Handle("nodes.select", func(w *state.World, p any) (any, error) {
		name, _ := p.(string)
		for i := range w.Nodes {
			w.Nodes[i].Selected = w.Nodes[i].Name == name
		}
		return map[string]any{"selected": name}, nil
	})
	st.Handle("nodes.select_many", func(w *state.World, p any) (any, error) {
		// Two shapes, because a selection arrives from a box drag as a list
		// and from the control socket as a name, and a caller should not have
		// to know which the interface happens to use.
		var names []string
		switch v := p.(type) {
		case []string:
			names = v
		case string:
			names = []string{v}
		}
		want := map[string]bool{}
		for _, n := range names {
			want[n] = true
		}
		for i := range w.Nodes {
			w.Nodes[i].Selected = want[w.Nodes[i].Name]
		}
		return map[string]any{"selected": names}, nil
	})
	st.Handle("nodes.add_to_selection", func(w *state.World, p any) (any, error) {
		var names []string
		switch v := p.(type) {
		case []string:
			names = v
		case string:
			names = []string{v}
		}
		n := 0
		for _, name := range names {
			for i := range w.Nodes {
				if w.Nodes[i].Name == name {
					w.Nodes[i].Selected = true
					n++
				}
			}
		}
		return map[string]any{"added": n}, nil
	})
	st.Handle("links.recompute", func(w *state.World, _ any) (any, error) {
		// Also the verb a node move calls when the drag ends, so dragging a
		// node across a country does not recompute every frame of the drag.
		s.warm(st, len(w.Nodes))
		return map[string]any{"warming": true}, nil
	})
	st.Handle("links.set", func(w *state.World, p any) (any, error) {
		links, _ := p.([]state.Link)
		w.Links = links
		w.Say(fmt.Sprintf("%d links, weighted by the weaker direction's margin",
			len(links)))
		return map[string]any{"links": len(links)}, nil
	})
	st.Handle("job.progress", func(w *state.World, p any) (any, error) {
		j, _ := p.(state.Job)
		for i := range w.Jobs {
			if w.Jobs[i].ID == j.ID {
				w.Jobs[i] = j
				return nil, nil
			}
		}
		w.Jobs = append(w.Jobs, j)
		return nil, nil
	})
	st.Handle("nodes.move", func(w *state.World, p any) (any, error) {
		m, _ := p.(map[string]any)
		name, _ := m["name"].(string)
		lat, _ := m["lat"].(float64)
		lon, _ := m["lon"].(float64)
		for i := range w.Nodes {
			if w.Nodes[i].Name == name {
				w.Nodes[i].Lat, w.Nodes[i].Lon = lat, lon
				return map[string]any{"name": name, "lat": lat, "lon": lon}, nil
			}
		}
		return nil, fmt.Errorf("no node named %q", name)
	})
	st.Handle("sim.inject", func(w *state.World, p any) (any, error) {
		// Originating a packet without firmware on the node. The engine
		// delivers to everything in range regardless, so this exercises the
		// radio model and the map's traffic layer; what it does not exercise
		// is relaying, which is a firmware behaviour and needs a firmware.
		if s.eng == nil {
			return nil, fmt.Errorf("no simulation")
		}
		at := 0
		if name, ok := p.(string); ok && name != "" {
			for i := range w.Nodes {
				if w.Nodes[i].Name == name {
					at = i
				}
			}
		} else {
			for i := range w.Nodes {
				if w.Nodes[i].Selected {
					at = i
					break
				}
			}
		}
		s.eng.Inject(at, []byte("msim-map-trace"))
		w.Say("injected a packet at " + w.Nodes[at].Name)
		return map[string]any{"at": w.Nodes[at].Name}, nil
	})
	st.Handle("coverage.compute", func(w *state.World, p any) (any, error) {
		// The selected node unless told otherwise, because "coverage from
		// here" is what somebody means when they have just clicked a node.
		at := -1
		if name, ok := p.(string); ok && name != "" {
			for i := range w.Nodes {
				if w.Nodes[i].Name == name {
					at = i
				}
			}
		} else {
			for i := range w.Nodes {
				if w.Nodes[i].Selected {
					at = i
					break
				}
			}
		}
		if at < 0 || at >= len(s.nodes) {
			return nil, fmt.Errorf("no node selected to compute coverage from")
		}
		n := s.nodes[at]
		w.Say("computing coverage from " + n.Name)
		// On a worker: 25,600 terrain profiles is not a thing to do on the
		// goroutine that owns the world.
		go func() {
			ctx := context.Background()
			cov, err := s.coverageFor(ctx, n, 60)
			if err != nil {
				_, _ = st.Do(ctx, "coverage.failed", err.Error())
				return
			}
			_, _ = st.Do(ctx, "coverage.set", cov)
		}()
		return map[string]any{"from": n.Name}, nil
	})
	st.Handle("coverage.set", func(w *state.World, p any) (any, error) {
		cov, _ := p.(*state.Coverage)
		w.Coverage = cov
		if cov == nil {
			return nil, nil
		}
		// Say how much of it is ignorance rather than absence. A raster
		// computed with no elevation data is mostly a statement about the
		// tile cache, and it looks exactly like a statement about radio.
		if cov.NoDataCells > 0 {
			w.Say(fmt.Sprintf("coverage from %s: %d of %d cells had no elevation data",
				cov.Node, cov.NoDataCells, cov.Cells))
		} else {
			w.Say("coverage from " + cov.Node)
		}
		return map[string]any{"node": cov.Node}, nil
	})
	st.Handle("terrain.shade", func(w *state.World, p any) (any, error) {
		box, _ := p.([4]float64)
		if box == [4]float64{} {
			return nil, fmt.Errorf("no view to shade")
		}
		go func() {
			ctx := context.Background()
			sh, err := s.hillshade(box[0], box[1], box[2], box[3])
			if err != nil || sh == nil {
				_, _ = st.Do(ctx, "terrain.shade_failed", nil)
				return
			}
			_, _ = st.Do(ctx, "terrain.shade_set", sh)
		}()
		return map[string]any{"shading": true}, nil
	})
	st.Handle("terrain.shade_set", func(w *state.World, p any) (any, error) {
		sh, _ := p.(*state.Coverage)
		w.Shade = sh
		if sh != nil && sh.NoDataCells > sh.Cells/2 {
			w.Say("terrain shading: most of this view has no elevation data cached")
		}
		return nil, nil
	})
	st.Handle("terrain.shade_failed", func(w *state.World, _ any) (any, error) {
		w.Shade = nil
		w.Say("terrain shading: no elevation data for this view")
		return nil, nil
	})
	st.Handle("waterfall.capture", func(w *state.World, p any) (any, error) {
		at := -1
		if name, ok := p.(string); ok && name != "" {
			for i := range w.Nodes {
				if w.Nodes[i].Name == name {
					at = i
				}
			}
		} else {
			for i := range w.Nodes {
				if w.Nodes[i].Selected {
					at = i
					break
				}
			}
		}
		// Captured here rather than on a worker: this is one 200 ms window of
		// samples through one FFT, not a national raster, and doing it inline
		// means the capture is of the instant that was asked for rather than
		// of whenever a goroutine got round to it.
		img, note := s.capture(context.Background(), at)
		w.Waterfall, w.WaterfallNote = img, note
		if note != "" {
			w.Say(note)
		}
		return map[string]any{"captured": img != nil}, nil
	})
	st.Handle("coverage.clear", func(w *state.World, _ any) (any, error) {
		w.Coverage = nil
		return nil, nil
	})
	st.Handle("coverage.failed", func(w *state.World, p any) (any, error) {
		msg, _ := p.(string)
		w.Say("coverage failed: " + msg)
		return nil, nil
	})
	st.Handle("session.describe", func(w *state.World, _ any) (any, error) {
		return map[string]any{
			"nodes": len(w.Nodes), "seed": w.Seed, "now_ms": w.NowMs,
			"playing": w.Playing,
		}, nil
	})
}
