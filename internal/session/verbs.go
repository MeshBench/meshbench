// Control verbs, registered on the store.
//
// The parity test in 12.9 is generated from what is registered here rather
// than from a list in a document, which was already wrong by three verbs.
package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/MeshBench/meshbench/internal/gui/state"
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
	registerLogs(st, s)
	registerRunKind(st, s)
	registerNodeWindow(st, s)
	registerFirmwareLibrary(st, s)
	registerFleet(st, s)
	registerGPU(st, s)
	registerTileCache(st, s)
	registerTerrainPrefetch(st, s)
	registerBasemap(st, s)
	registerPacket(st, s)
	registerLinkProfile(st, s)
	registerImport(st, s)
	registerBoundary(st, s)
	registerPlanningVerbs(st, s)
	registerSchedule(st, s)
	registerValidate(st, s)
	registerUIVerbs(st, s)
	registerCapture(st, s)
	registerCompanion(st, s)
	registerMeshCLI(st, s)
	registerProvisioningSettings(st, s)
	registerProvisioningRules(st, s)
	registerProvisioningKeys(st, s)
	registerProvisioningContext(st, s)
	registerProvisioningPreview(st, s)
	registerBoundaryGeoJSON(st, s)
	registerRadioReconcile(st, s)
	registerExperiment(st, s)
	registerExperimentDone(st, s)
	registerCoverageCombined(st, s)
	st.Handle("project.open", func(w *state.World, p any) (any, error) {
		path := soleString(p)
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
			// A sweep cell paces its own engine against wall time and counts on
			// that pacing. Stepping it again here would not make the run
			// faster - it would make its airtime accounting wrong - so while a
			// cell owns the clock this only reads.
			if !s.benchOwnsTheClock() {
				_ = s.eng.Step(context.Background())
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
			// Every open console gets the clock before the step that will
			// produce the lines it stamps.
			for _, buf := range s.consoles {
				buf.SetNow(w.NowMs)
			}
			// And the one being looked at is re-read after it. A reply lands
			// in the buffer when the firmware's loop runs, which is now -
			// published only on the next console.type, every answer appeared
			// one command late, which reads as a console that does not answer.
			// Every watched console gets re-read after the step that will have
			// produced the lines it stamps - not only whichever one was most
			// recently typed into, so two open node windows both stay live.
			for name := range w.Consoles {
				if buf, ok := s.consoles[name]; ok {
					setConsole(w, name, buf.Snapshot())
				}
			}
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
			s.refreshOpenPacket(w)
			w.Events, w.EventTotal = s.eventTail(eventTail)
			w.Counts = s.eventCounts()
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
	// job.done removes one, because a progress bar that never goes away is a
	// worse lie than no progress bar.
	st.Handle("job.done", func(w *state.World, p any) (any, error) {
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
	registerCoverageVerbs(st, s)
	registerBudgetVerbs(st, s)

	registerBenchVerbs(st, s)
	registerSweepVerbs(st, s)
	registerImportFeedVerbs(st, s)
	registerNodeFirmwareVerbs(st, s)
	st.Handle("session.describe", func(w *state.World, _ any) (any, error) {
		return map[string]any{
			"nodes": len(w.Nodes), "seed": w.Seed, "now_ms": w.NowMs,
			"playing": w.Playing,
		}, nil
	})
}
