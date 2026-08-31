// Control verbs, registered on the store.
//
// The parity test in 12.9 is generated from what is registered here rather
// than from a list in a document, which was already wrong by three verbs.
package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// Register wires the control verbs onto the store. Only the few the new
// UI needs so far; the rest arrive as their panels do, and the parity test in
// 12.9 is generated from what is registered here.
func Register(st *state.Store, s *Sim) {
	registerSimControl(st, s)
	registerJournal(st, s)
	registerUI(st, s)
	registerMapCamera(st, s)
	registerExcessLoss(st, s)
	registerConsole(st, s)
	registerLogs(st, s)
	registerRunKind(st, s)
	registerUnverifiedWiring(st, s)
	registerNodeWindow(st, s)
	registerFirmwareWindow(st, s)
	registerFirmwareLibrary(st, s)
	registerFirmwareScan(st, s)
	registerFirmwareDetail(st, s)
	registerFirmwareBuild(st, s)
	registerFirmwareBuildResults(st, s)
	registerNodesBulk(st, s)
	registerGPU(st, s)
	registerTileCache(st, s)
	registerTerrainPrefetch(st, s)
	registerBasemap(st, s)
	registerPacket(st, s)
	registerImport(st, s)
	registerSchedule(st, s)
	registerUIVerbs(st, s)
	registerKeepAbove(st, s)
	registerPresetsAndPlace(st, s)
	registerCompanion(st, s)
	registerMeshCLI(st, s)
	registerRFMode(st, s)
	registerRFRealism(st, s)
	registerRFEnvironment(st, s)
	registerRadioReconcile(st, s)
	registerExperiment(st, s)
	registerExperimentDone(st, s)
	runDomainRegistrars(st, s)
	registerCheckpoint(st, s)
	// project.new: an empty network, to build one by hand.
	//
	// The same path as opening, with nothing in it. Everything downstream -
	// the engine, the tick, the readouts - is built the same way, because a
	// blank network that took a different route through this would be a
	// second kind of session with its own set of things that were not set.
	st.Handle("project.new", func(w *state.World, p any) (any, error) {
		s.installFn(st, w, Loaded{}, "a blank network")
		// Somewhere to start, if a place was named.
		//
		// A blank network with no nodes has nothing to frame the map on, and
		// a map framed on nothing is the middle of the Atlantic. The place
		// is looked up the same way a study area is, so "Fife" means the
		// same thing in both.
		place, _ := stringField(p, "place")
		if strings.TrimSpace(place) == "" {
			w.Say("a blank network - place nodes with the map's place tool")
			return map[string]any{"nodes": 0}, nil
		}
		w.Say("looking up " + place)
		// The place becomes the study area, and the map is framed on it.
		//
		// Both, because they are the same wish: somebody starting a network
		// in Fife wants to see Fife and wants what they build measured
		// against it. The search runs through the same verbs the Import
		// panel uses, so a place means the same thing everywhere - and a
		// name already searched for is taken from what that search found
		// rather than asked of the geocoder twice.
		go func() {
			ctx := context.Background()
			if !s.knowsArea(place) {
				if _, err := st.Do(ctx, "boundary.set",
					map[string]any{"query": place}); err != nil {
					_, _ = st.Do(ctx, "ui.said", "no place called "+place+
						" - the map stays where it was")
					return
				}
			}
			if _, err := st.Do(ctx, "boundary.accept",
				map[string]any{"name": place}); err != nil {
				_, _ = st.Do(ctx, "ui.said", err.Error())
				return
			}
			// Framed by the map itself, which is the only thing that knows
			// how many pixels it has to frame it in.
			_, _ = st.Do(ctx, "map.fit", nil)
		}()
		return map[string]any{"nodes": 0, "place": place}, nil
	})
	st.Handle("project.open", func(w *state.World, p any) (any, error) {
		path := soleString(p)
		f, err := LoadFixture(path)
		if err != nil {
			return nil, err
		}
		s.installFn(st, w, f, path)
		w.Say(fmt.Sprintf("opened %s: %d nodes, %d links, %d areas",
			path, len(f.nodes), len(w.Links), len(f.areas)))
		return map[string]any{
			"opened": path, "nodes": len(f.nodes), "links": len(w.Links),
		}, nil
	})

	// install puts a loaded network in place: the world's copy of it, the
	// engine, and the tick that drives both.
	installBody := func(st *state.Store, w *state.World, f Loaded, path string) {
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
		// Always through warm, never a shortcut off a carried matrix: a
		// carry-in - in-process or from disk under this geometry's
		// fingerprint - primes the cache but is not proof every pair is
		// still good, since a firmware node's real radio configuration can
		// diverge from the baseline figures the matrix was measured
		// against. warm is fast when the carry-in already covers most of
		// it - pathLoss checks the cache first - and it is the only thing
		// that gets to say the matrix is complete.
		s.warm(st, len(f.scene))
		// One engine step per tick. Step is the engine's own unit of time
		// and takes its size from the config, so the store paces it rather
		// than redefining it.
		index := map[string]int{}
		for i, n := range f.nodes {
			index[n.Name] = i
		}
		w.Tick = func(uint32) {
			w.FirmwareRunning, w.FirmwareStarting = s.firmwareCount(), s.starting.Load()
			// A sweep cell paces its own engine against wall time and counts on
			// that pacing. Stepping it again here would not make the run
			// faster - it would make its airtime accounting wrong - so while a
			// cell owns the clock this only reads.
			if !s.benchOwnsTheClock() {
				_ = s.eng.Step(context.Background())
				// Whatever the schedule is due to say at this moment of
				// simulated time. After the step, so a send lands on a mesh
				// whose clock has already reached its moment.
				s.fireDueSends(w)
			}
			// A rebuild anywhere leaves the matrix cold; this is the one
			// place every run passes through, so it is where the warm is
			// made certain rather than remembered at nine call sites.
			if s.cold && len(s.nodes) >= 10 {
				s.warm(st, len(s.nodes))
			}
			// A pair profiled outside a warm is a tick that just paid a
			// terrain walk it was not expecting to - the stall that once
			// read as the run having simply stopped. Said only once the
			// pairs are no longer moving because a warm is expected to be
			// mid-flight; the warming chip already covers that case.
			if eng := s.liveEngine(); eng != nil {
				if lp := eng.LiveProfiles(); lp > s.lastLiveProfiles {
					delta := lp - s.lastLiveProfiles
					s.lastLiveProfiles = lp
					if !s.warming() {
						w.Say(fmt.Sprintf(
							"recomputed %d link(s) live, mid-run - a node's radio "+
								"configuration changed since the last warm; "+
								"\"rewarm links\" clears it", delta))
					}
				}
				// A crashed node used to freeze silently - runFirmware
				// returned on the first Bridge call that failed, so nothing
				// after it in node order ever ticked again, and nobody was
				// told. It is skipped now instead; this is where that gets
				// said, so "why has it gone quiet" has an answer.
				if down := eng.FirmwareFailures(); len(down) > 0 {
					has := "has"
					if len(down) > 1 {
						has = "have"
					}
					w.Say(fmt.Sprintf(
						"%s stopped answering and %s been dropped from this run - "+
							"its firmware process is gone; the rest of the mesh keeps going",
						strings.Join(down, ", "), has))
				}
			}
			w.NowMs = s.liveEngine().NowMs()
			// Anything a split-out domain must re-describe every step - a
			// client attaching to a served observer is not a verb, so the
			// fact is re-read here rather than trusted from the last one.
			runTicks(s, w)
			// Every open console gets the clock before the step that will
			// produce the lines it stamps.
			for _, buf := range s.consoles {
				buf.SetNow(w.NowMs)
			}
			// And the one being looked at is re-read after it. A reply lands
			// in the buffer when the firmware's loop runs, which is now -
			// published only on the next console.type, every answer appeared
			// one command late, which reads as a console that does not answer.
			if w.ConsoleNode != "" {
				if buf, ok := s.consoles[w.ConsoleNode]; ok {
					w.Console = buf.Snapshot()
				}
			}
			// And the output pane, for the same reason. Its source is a file
			// the emulator is still writing, so a pane that read it once shows
			// a board that stopped talking the moment it was opened.
			if len(w.Outputs) > 0 {
				s.refreshOutput(w)
			}
			w.RFMode = string(rfModeOf(s.rfMode))
			// The calibration the model is running with, and whether it was
			// fitted or left at the default. A margin's provenance travels
			// with the margin or the panel showing it has to guess.
			w.ExcessLossDB, w.Calibrated = s.excessLossDB, s.excessSet
			w.RFRealism = s.realism
			w.RFEnvironment = s.envDir
			w.CoverageCells = s.covCells
			// The readouts - tail conversion, per-node bridge stats, trail
			// scan - are for human eyes, and eyes read at ten hertz. Doing
			// them on every 10 ms step was a tenth of the tick spent
			// re-describing a table nobody could have re-read yet - and the
			// tick paces the engine's clock, so readout cost was simulation
			// slowness. A paused or hand-stepped run refreshes every tick,
			// because a person stepping once is looking straight at it.
			if !w.Playing || time.Since(s.lastReadout) >= 100*time.Millisecond {
				s.lastReadout = time.Now()
				s.refreshReadouts(w, index)
			}
		}

		// The RF facts are published on ticks, and ticks wait for play - but
		// the chrome's mode chip and caveat line are read the moment the
		// window opens, and a waveform preference wearing a calculated label
		// is a lie about what the next run will do.
		w.RFMode = string(rfModeOf(s.rfMode))
		w.KeepAbove = s.keepAbove()
		w.RFRealism = s.realism
		w.RFEnvironment = s.envDir
		w.CoverageCells = s.covCells
	}
	s.installFn = installBody
	registerClockVerbs(st, s)
	st.Handle("nodes.select", func(w *state.World, p any) (any, error) {
		name := soleString(p)
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
	st.HandleInternal("links.set", func(w *state.World, p any) (any, error) {
		links, ok := p.([]state.Link)
		if !ok {
			return nil, wrongCallback("links.set")
		}
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
	st.HandleInternal("job.progress", func(w *state.World, p any) (any, error) {
		j, ok := p.(state.Job)
		if !ok {
			return nil, wrongCallback("job.progress")
		}
		for i := range w.Jobs {
			if w.Jobs[i].ID == j.ID {
				// A progress update carries counts, not closures. The callback
				// that reports "412 of 500" has no way to rebuild the cancel
				// function whoever started the job registered, so replacing the
				// row wholesale threw away the only handle anybody had on
				// stopping it - which is why state.Job.Cancel existed for
				// months and could never once be called.
				if j.Cancel == nil {
					j.Cancel = w.Jobs[i].Cancel
				}
				w.Jobs[i] = j
				return nil, nil
			}
		}
		w.Jobs = append(w.Jobs, j)
		return nil, nil
	})
	// job.cancel stops one, where whoever started it left a way to.
	//
	// Refusing by name rather than silently doing nothing: a job with no
	// cancel is one that cannot be interrupted safely, and an operator who
	// asked deserves to be told that rather than left watching a bar that
	// carries on.
	st.Handle("job.cancel", func(w *state.World, p any) (any, error) {
		id := soleString(p)
		if m, ok := p.(map[string]any); ok {
			id, _ = m["id"].(string)
		}
		if id == "" {
			return nil, fmt.Errorf("job.cancel needs an id")
		}
		for i := range w.Jobs {
			if w.Jobs[i].ID != id {
				continue
			}
			if w.Jobs[i].Cancel == nil {
				return nil, fmt.Errorf("%q cannot be stopped once it has started", id)
			}
			w.Jobs[i].Cancel()
			// Said, and left on screen saying it: the work stops when its
			// context notices, which is not this instant, and a row that
			// vanished on the press would claim otherwise.
			w.Jobs[i].What = "stopping: " + w.Jobs[i].What
			w.Say("stopping " + id)
			return map[string]any{"stopping": id}, nil
		}
		return nil, fmt.Errorf("no job called %q is running", id)
	})

	// job.done removes one, because a progress bar that never goes away is a
	// worse lie than no progress bar.
	st.HandleInternal("job.done", func(w *state.World, p any) (any, error) {
		id := soleString(p)
		for i := range w.Jobs {
			if w.Jobs[i].ID == id {
				w.Jobs = append(w.Jobs[:i], w.Jobs[i+1:]...)
				break
			}
		}
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
				// The physics moves with the marker: cached losses for this
				// node are forgotten, so an attached SDR client hears the
				// new position on the next window.
				if s.eng != nil {
					s.eng.SetNodePosition(i, lat, lon)
				}
				return map[string]any{"name": name, "lat": lat, "lon": lon}, nil
			}
		}
		return nil, noSuchNode(name)
	})
	st.Handle("sim.inject", func(w *state.World, p any) (any, error) {
		// Originating a packet without firmware on the node. The engine
		// delivers to everything in range regardless, so this exercises the
		// radio model and the map's traffic layer; what it does not exercise
		// is relaying, which is a firmware behaviour and needs a firmware.
		if s.eng == nil {
			return nil, ErrNoSimulation
		}
		// A network with no nodes has nowhere to originate from. It used to
		// be unreachable - every session began with a fixture - and starting
		// a blank network made it a state somebody can be in, where this
		// indexed an empty slice and took the process with it.
		if len(w.Nodes) == 0 {
			return nil, fmt.Errorf("no nodes to originate from - place one first")
		}
		at := 0
		if name := soleString(p); name != "" {
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
	registerBudgetVerbs(st, s)

	registerBenchVerbs(st, s)
	registerSweepVerbs(st, s)
	registerImportFeedVerbs(st, s)
	registerNodeFirmwareVerbs(st, s)
	registerNodeOutput(st, s)
	registerNodeCard(st, s)
	registerNodeOutputWindow(st, s)
	st.Handle("session.describe", func(w *state.World, _ any) (any, error) {
		return map[string]any{
			"nodes": len(w.Nodes), "seed": w.Seed, "now_ms": w.NowMs,
			"playing": w.Playing,
		}, nil
	})
	// Last, because it reads what everything above has registered.
	excludeInternalFromJournal(st)
}
