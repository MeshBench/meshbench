// Control verbs, registered on the store.
//
// The parity test in 12.9 is generated from what is registered here rather
// than from a list in a document, which was already wrong by three verbs.
package session

import (
	"context"
	"fmt"
	"time"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

// Register wires the control verbs onto the store. Only the few the new
// UI needs so far; the rest arrive as their panels do, and the parity test in
// 12.9 is generated from what is registered here.
func Register(st *state.Store, s *Sim) {
	registerSimControl(st, s)
	registerInventory(st, s)
	registerUI(st, s)
	registerMapCamera(st, s)
	registerExcessLoss(st, s)
	registerConsole(st, s)
	registerRunKind(st, s)
	registerNodeWindow(st, s)
	registerFirmwareLibrary(st, s)
	registerFleet(st, s)
	registerImport(st, s)
	registerBoundary(st, s)
	registerPlanningVerbs(st, s)
	registerSchedule(st, s)
	registerValidate(st, s)
	registerUIVerbs(st, s)
	registerCapture(st, s)
	registerCompanion(st, s)
	registerCoverageCombined(st, s)
	st.Handle("project.open", func(w *state.World, p any) (any, error) {
		path, _ := p.(string)
		f, err := LoadFixture(path)
		if err != nil {
			return nil, err
		}
		w.Nodes, w.Areas, w.MarginKm = f.nodes, f.areas, f.margin
		w.Sends, w.Assertions = f.sends, f.assertions
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
			w.FirmwareRunning, w.FirmwareStarting = s.firmwareCount(), s.starting.Load()
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
			w.Stats = s.nodeStats(w.Events)
			if s.history == nil {
				s.history = newNodeHistory()
			}
			s.history.record(w.Stats)
			for i := range w.Nodes {
				if w.Nodes[i].Selected {
					w.Series = s.history.seriesFor(w.Nodes[i].Name)
					break
				}
			}
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
		// The budget is about a link, so it cannot exist before the links do.
		// Asking for it on a timer at startup gave an empty panel, because
		// measuring 48,000 path losses takes longer than any timer worth
		// guessing at.
		at := -1
		for i := range w.Nodes {
			if w.Nodes[i].Selected {
				at = i
				break
			}
		}
		w.Budgets = s.budgetsFor(at, links)
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
	st.Handle("budget.for_selection", func(w *state.World, _ any) (any, error) {
		at := -1
		for i := range w.Nodes {
			if w.Nodes[i].Selected {
				at = i
				break
			}
		}
		w.Budgets = s.budgetsFor(at, w.Links)
		return map[string]any{"budgets": len(w.Budgets)}, nil
	})
	st.Handle("energy.for_selection", func(w *state.World, _ any) (any, error) {
		at := -1
		for i := range w.Nodes {
			if w.Nodes[i].Selected {
				at = i
				break
			}
		}
		if at < 0 || at >= len(s.nodes) {
			return nil, fmt.Errorf("no node selected")
		}
		// The duty cycle the run measured, not one typed into a form.
		duty := 0.0
		for _, v := range w.Scores {
			if v.Name == w.Nodes[at].Name {
				duty = v.DutyCyclePct
			}
		}
		e, err := EnergyFor(s.nodes[at], duty)
		if err != nil {
			return nil, err
		}
		w.Energy = e
		w.Say(fmt.Sprintf("%s: worst state of charge %.0f%% on day %d, %d dead days",
			e.Node, e.WorstSoC*100, e.WorstDay, e.DeadDays))
		return map[string]any{"node": e.Node}, nil
	})
	st.Handle("bench.serve", func(w *state.World, p any) (any, error) {
		m, _ := p.(map[string]any)
		name, _ := m["node"].(string)
		kind, _ := m["kind"].(string)
		if name == "" {
			// The first companion, because "give me an endpoint" is what
			// somebody means before they have chosen one.
			for i := range w.Nodes {
				if w.Nodes[i].Kind == "companion" {
					name = w.Nodes[i].Name
					break
				}
			}
		}
		ep, err := s.serve(name, kind)
		if err != nil {
			w.Say("could not serve " + name + ": " + err.Error())
			return nil, err
		}
		w.Endpoints = s.endpoints()
		w.Say(ep.Node + " is at " + ep.Addr)
		return map[string]any{"node": ep.Node, "addr": ep.Addr}, nil
	})
	st.Handle("bench.drop", func(w *state.World, _ any) (any, error) {
		n := s.dropClients()
		w.Endpoints = s.endpoints()
		w.Say(fmt.Sprintf("dropped %d client connection(s)", n))
		return map[string]any{"dropped": n}, nil
	})
	st.Handle("bench.stray", func(w *state.World, _ any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no simulation")
		}
		for i := range w.Nodes {
			if w.Nodes[i].Kind == "companion" {
				s.eng.Inject(i, []byte("msim-stray"))
				w.Say("injected an unexpected frame at " + w.Nodes[i].Name)
				return map[string]any{"at": w.Nodes[i].Name}, nil
			}
		}
		return nil, fmt.Errorf("no companion to inject at")
	})
	st.Handle("bench.refresh", func(w *state.World, _ any) (any, error) {
		// Whether a client is attached changes without anything asking, so
		// the panel refreshes rather than assuming what it drew is still true.
		w.Endpoints = s.endpoints()
		return nil, nil
	})
	st.Handle("run.save", func(w *state.World, p any) (any, error) {
		name, _ := p.(string)
		if name == "" {
			name = "run"
		}
		// Saved from the snapshot rather than from the world, so what is
		// recorded is exactly what was on screen when somebody pressed save.
		path, err := SaveRun(name, st.Snapshot(), BuildOf(s))
		if err != nil {
			return nil, err
		}
		w.Say("saved " + path)
		return map[string]any{"path": path}, nil
	})
	st.Handle("sweep.run", func(w *state.World, _ any) (any, error) {
		if s.eng == nil || len(s.nodes) == 0 {
			return nil, fmt.Errorf("no network to sweep")
		}
		node := FirstCompanion(s.nodes)
		for i := range w.Nodes {
			if w.Nodes[i].Selected {
				node = w.Nodes[i].Name
				break
			}
		}
		plan := DefaultSweep(node)
		total := len(plan.Arms) * len(plan.Seeds)
		w.Say(fmt.Sprintf("sweeping %d arms over %d seeds from %s",
			len(plan.Arms), len(plan.Seeds), node))
		// On a worker, with progress: this is twenty-four short simulations
		// and the interface has to stay usable while they run.
		go func() {
			ctx := context.Background()
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "sweep", What: "sweeping offered load", Total: total})
			m := s.runSweep(ctx, plan, func(done, of int) {
				_, _ = st.Do(ctx, "job.progress", state.Job{
					ID: "sweep", What: "sweeping offered load",
					Done: done, Total: of})
			})
			_, _ = st.Do(ctx, "sweep.set", m)
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "sweep", What: "sweeping offered load",
				Done: total, Total: total, Finished: true})
		}()
		return map[string]any{"arms": len(plan.Arms), "seeds": len(plan.Seeds)}, nil
	})
	st.Handle("sweep.set", func(w *state.World, p any) (any, error) {
		m, _ := p.(*state.Matrix)
		w.Matrix = m
		if m != nil {
			w.Say(fmt.Sprintf("swept %d arms over %d seeds", len(m.Arms), len(m.Seeds)))
		}
		return nil, nil
	})
	st.Handle("plan.routes", func(w *state.World, _ any) (any, error) {
		var picked []string
		for i := range w.Nodes {
			if w.Nodes[i].Selected {
				picked = append(picked, w.Nodes[i].Name)
			}
		}
		if len(picked) < 2 {
			return nil, fmt.Errorf("select two nodes to plan between")
		}
		from, to := picked[0], picked[len(picked)-1]
		w.Say("searching for routes between " + from + " and " + to)
		go func() {
			ctx := context.Background()
			routes, err := s.routesBetween(from, to)
			if err != nil {
				_, _ = st.Do(ctx, "plan.failed", err.Error())
				return
			}
			_, _ = st.Do(ctx, "plan.set", routes)
		}()
		return map[string]any{"from": from, "to": to}, nil
	})
	st.Handle("plan.set", func(w *state.World, p any) (any, error) {
		routes, _ := p.([]state.Route)
		w.Routes = routes
		w.Say(fmt.Sprintf("%d route(s) found", len(routes)))
		return map[string]any{"routes": len(routes)}, nil
	})
	st.Handle("plan.failed", func(w *state.World, p any) (any, error) {
		msg, _ := p.(string)
		w.Say("planning: " + msg)
		return nil, nil
	})
	st.Handle("import.describe", func(w *state.World, p any) (any, error) {
		url, _ := p.(string)
		w.Say("fetching " + url)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			im, err := ImportFrom(ctx, url)
			if err != nil {
				_, _ = st.Do(context.Background(), "import.failed", err.Error())
				return
			}
			_, _ = st.Do(context.Background(), "import.set", im)
		}()
		return map[string]any{"url": url}, nil
	})
	st.Handle("import.set", func(w *state.World, p any) (any, error) {
		im, _ := p.(*state.Import)
		w.Import = im
		if im != nil {
			w.Say(fmt.Sprintf(
				"%s: %d records, %d importable, %d with no position, %d placed loosely",
				im.URL, im.Records, im.Nodes, im.SkippedNoPosition, im.Uncertain))
		}
		return nil, nil
	})
	st.Handle("import.failed", func(w *state.World, p any) (any, error) {
		msg, _ := p.(string)
		w.Say("import failed: " + msg)
		return nil, nil
	})
	st.Handle("feed.pull", func(w *state.World, p any) (any, error) {
		url, _ := p.(string)
		if url == "" {
			return nil, fmt.Errorf("no deployment to pull from")
		}
		w.Say("pulling recent receptions from " + url)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			obs, err := PullObserved(ctx, url, time.Hour)
			if err != nil {
				// Its own message, not the import one: a deployment can
				// publish nodes and not receptions, and telling somebody the
				// import failed when the import worked sends them to the
				// wrong place.
				_, _ = st.Do(context.Background(), "feed.failed", err.Error())
				return
			}
			_, _ = st.Do(context.Background(), "feed.set", obs)
		}()
		return map[string]any{"url": url}, nil
	})
	st.Handle("feed.failed", func(w *state.World, p any) (any, error) {
		msg, _ := p.(string)
		w.Say("no live feed: " + msg)
		return nil, nil
	})
	st.Handle("feed.set", func(w *state.World, p any) (any, error) {
		obs, _ := p.([]state.Observed)
		w.Observed = obs
		// Residuals fall out of the same data, so they are computed here
		// rather than behind a second button somebody has to know to press.
		w.Residuals = s.residualsOf(obs, w.Links, w.Nodes)
		w.Say(fmt.Sprintf("%d receptions; %d matched a link in this scenario",
			len(obs), w.Residuals.Matched))
		return map[string]any{"receptions": len(obs)}, nil
	})
	st.Handle("sim.toggle", func(w *state.World, _ any) (any, error) {
		// One control for both, because play and pause are one thought.
		w.Playing = !w.Playing
		if w.Playing {
			w.Say("playing")
		} else {
			w.Say("paused")
		}
		return map[string]any{"playing": w.Playing}, nil
	})
	st.Handle("sim.step", func(w *state.World, _ any) (any, error) {
		// One step whether or not it is playing: stepping a paused simulation
		// is the whole point of a step control.
		if w.Tick != nil {
			w.Tick(0)
		}
		w.Say(fmt.Sprintf("stepped to %.2f s", float64(w.NowMs)/1000))
		return map[string]any{"now_ms": w.NowMs}, nil
	})
	st.Handle("sim.faster", func(w *state.World, _ any) (any, error) {
		st.SetStepMs(st.StepMs() * 2)
		w.Say(fmt.Sprintf("%d ms per tick", st.StepMs()))
		return map[string]any{"step_ms": st.StepMs()}, nil
	})
	st.Handle("sim.slower", func(w *state.World, _ any) (any, error) {
		st.SetStepMs(st.StepMs() / 2)
		w.Say(fmt.Sprintf("%d ms per tick", st.StepMs()))
		return map[string]any{"step_ms": st.StepMs()}, nil
	})
	st.Handle("firmware.start", func(w *state.World, _ any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no network Loaded")
		}
		w.Say("starting firmware on every node")
		s.startFirmware(st, w.Seed)
		return map[string]any{"starting": true}, nil
	})
	st.Handle("firmware.started", func(w *state.World, _ any) (any, error) {
		w.Say(fmt.Sprintf("%d nodes running firmware", s.firmwareCount()))
		return map[string]any{"running": s.firmwareCount()}, nil
	})
	st.Handle("firmware.failed", func(w *state.World, p any) (any, error) {
		msg, _ := p.(string)
		w.Say("firmware: " + msg)
		return nil, nil
	})
	st.Handle("firmware.state", func(w *state.World, _ any) (any, error) {
		return map[string]any{
			"running": s.firmwareCount(), "nodes": len(w.Nodes),
			"starting": s.starting.Load(),
		}, nil
	})
	st.Handle("nodes.stats", func(w *state.World, _ any) (any, error) {
		// Also on demand, because a paused simulation still costs memory and
		// somebody looking at the node view has usually just paused it.
		w.Stats = s.nodeStats(w.Events)
		return map[string]any{"nodes": len(w.Stats)}, nil
	})
	st.Handle("node.stop", func(w *state.World, p any) (any, error) {
		name, _ := p.(string)
		if err := s.stopNode(name); err != nil {
			return nil, err
		}
		w.Stats = s.nodeStats(w.Events)
		w.Say("stopped " + name)
		return map[string]any{"stopped": name}, nil
	})
	st.Handle("node.start", func(w *state.World, p any) (any, error) {
		name, _ := p.(string)
		if err := s.startNode(context.Background(), name, w.Seed); err != nil {
			return nil, err
		}
		w.Stats = s.nodeStats(w.Events)
		w.Say("started " + name)
		return map[string]any{"started": name}, nil
	})
	st.Handle("node.set_firmware", func(w *state.World, p any) (any, error) {
		m, _ := p.(map[string]any)
		name, _ := m["node"].(string)
		version, _ := m["version"].(string)
		// Applied, not just recorded: stop, provision, start. Firmware is
		// chosen when a node launches, so setting it on a running node changes
		// nothing until something restarts it.
		s.Reflash(context.Background(), st, name, version, w.Seed)
		w.Say(name + ": changing to " + version)
		return map[string]any{"node": name, "version": version}, nil
	})
	st.Handle("node.reflashed", func(w *state.World, p any) (any, error) {
		msg, _ := p.(string)
		w.Stats = s.nodeStats(w.Events)
		w.Say(msg)
		return nil, nil
	})
	st.Handle("node.reflash_failed", func(w *state.World, p any) (any, error) {
		msg, _ := p.(string)
		w.Stats = s.nodeStats(w.Events)
		w.Say("firmware change failed: " + msg)
		return nil, nil
	})
	st.Handle("node.set_firmware_only", func(w *state.World, p any) (any, error) {
		m, _ := p.(map[string]any)
		name, _ := m["node"].(string)
		version, _ := m["version"].(string)
		if err := s.setFirmware(name, version); err != nil {
			return nil, err
		}
		// Not applied until the node restarts, and said so rather than left to
		// be discovered: firmware is chosen at launch, and a version that
		// changes nothing until something else happens is the kind of control
		// somebody presses twice and then distrusts.
		w.Say(name + " will run " + version + " when it next starts")
		return map[string]any{"node": name, "version": version}, nil
	})
	st.Handle("node.provisioning", func(w *state.World, p any) (any, error) {
		name, _ := p.(string)
		lines, err := s.provisioningFor(name)
		if err != nil {
			return nil, err
		}
		w.Provisioning, w.ProvisioningNode = lines, name
		var cmds []string
		for _, l := range lines {
			if !l.Comment {
				cmds = append(cmds, l.Command)
			}
		}
		return map[string]any{"node": name, "commands": cmds}, nil
	})
	st.Handle("session.describe", func(w *state.World, _ any) (any, error) {
		return map[string]any{
			"nodes": len(w.Nodes), "seed": w.Seed, "now_ms": w.NowMs,
			"playing": w.Playing,
		}, nil
	})
}
