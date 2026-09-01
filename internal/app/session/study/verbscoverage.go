// Coverage, terrain shading and the waterfall: the three things the map can
// be asked to draw on top of itself, and the three that take long enough to
// need a job and a failure verb each.
package study

import (
	"context"
	"fmt"
	"math"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerCoverageVerbs(st *state.Store, s *session.Sim) {
	st.HandleSpec("coverage.compute", state.Spec{
		What: "raster what one node reaches over its own 60 km study square, " +
			"which is the question somebody has just asked by clicking a mast",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Primary: true,
				What: "the node to cover from; absent falls back to whichever " +
					"node is selected, and a name this network has not got is " +
					"refused rather than read as none"},
		},
		Returns: []string{"nodes", "started"},
		Answers: "It answers as soon as the job starts, with `nodes` at 1 for " +
			"the single station. The raster lands later through the internal " +
			"`coverage.set`, and a failure through `coverage.failed`. Every cell " +
			"is judged in both directions and they differ: the antenna's gain is " +
			"evaluated on the bearing and look angle to that cell, and a node " +
			"imported with position uncertainty carries that uncertainty into " +
			"the cell as slack. The margins are a best case, with no multipath " +
			"and no body loss in them, and cells with no cached elevation are " +
			"counted rather than coloured.",
		Example: &state.Example{
			Params: "West Lomond",
			What:   "see the ground one repeater serves",
		},
	}, func(w *state.World, p any) (any, error) {
		// The selected node unless told otherwise, because "coverage from
		// here" is what somebody means when they have just clicked a node.
		at := -1
		if name := session.SoleString(p); name != "" {
			for i := range w.Nodes {
				if w.Nodes[i].Name == name {
					at = i
				}
			}
			// A name that matched nothing used to leave this at -1 and be
			// reported as "no node selected", which sends whoever typed it
			// hunting for a selection rather than at their own spelling.
			if at < 0 {
				return nil, session.UnknownNames("coverage.compute", w.Nodes,
					[]string{name})
			}
		} else {
			for i := range w.Nodes {
				if w.Nodes[i].Selected {
					at = i
					break
				}
			}
		}
		if at < 0 || at >= len(s.Nodes()) {
			return nil, fmt.Errorf("no node selected to compute coverage from")
		}
		n := s.Nodes()[at]
		// The whole-map job with a station list of one: the GPU fold, the
		// buildings, the resolution knob and the percentage all arrive for
		// free, where the 160-cell tile walk this replaced had none of
		// them. The box is the node's 60 km study square, as it always was.
		dLat := 60.0 / 111.32
		dLon := 60.0 / (111.32 * math.Cos(n.Position.Lat*math.Pi/180))
		// A floor under the resolution knob: 800 cells is ~150 m over the
		// study box, which is what "coverage from here" is for - the knob
		// can still push it higher for everything at once.
		cells := coverageCells(s)
		if cells < 800 {
			cells = 800
		}
		return startCoverageMap(s, st, w, map[string]any{
			"station": n.Name, "cells": float64(cells),
			"south": n.Position.Lat - dLat, "north": n.Position.Lat + dLat,
			"west": n.Position.Lon - dLon, "east": n.Position.Lon + dLon,
		})
	})

	st.HandleInternalSpec("coverage.set", state.Spec{
		What: "take a finished raster into the snapshot and say how much of it " +
			"is ignorance rather than absence, since a raster computed with no " +
			"elevation looks exactly like a statement about radio",
		Returns: []string{"node"},
		Answers: "Answers nothing at all when it is handed a nil raster, which " +
			"is how the map is cleared.",
	}, func(w *state.World, p any) (any, error) {
		cov, ok := p.(*state.Coverage)
		if !ok {
			return nil, session.WrongCallback("coverage.set")
		}
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

	st.HandleSpec("terrain.shade", state.Spec{
		What: "hillshade the relief under one view, which is a tile fetch and a " +
			"pass over every cell in it, so it runs as a job that says out loud " +
			"that it is happening",
		Params: []state.Param{
			{Name: "view", Type: state.ParamArray, Required: true, Primary: true,
				What: "the borders to shade, as the four numbers south, north, " +
					"west, east; anything that is not four numbers, and four " +
					"that are all zero, is refused"},
		},
		Returns: []string{"shading"},
		Answers: "`shading: true` means the job started, not that anything has " +
			"been drawn. The shading lands later through the internal " +
			"`terrain.shade_set`, or `terrain.shade_failed` where the view has " +
			"no elevation to shade.",
		Example: &state.Example{
			Params: []any{56.0, 56.5, -3.6, -3.0},
			What:   "shade the relief across Fife",
		},
	}, func(w *state.World, p any) (any, error) {
		// Either shape: the map hands over the array it keeps, and anything
		// driving the workbench from outside has only JSON, which arrives as
		// a list. It took the array alone, so shading could not be asked for
		// from a script, a capture or a test at all.
		box, ok := p.([4]float64)
		if !ok {
			var got []float64
			switch v := p.(type) {
			case []float64:
				got = v
			case []any:
				for _, x := range v {
					f, isNum := x.(float64)
					if !isNum {
						break
					}
					got = append(got, f)
				}
			}
			if len(got) == 4 {
				box, ok = [4]float64{got[0], got[1], got[2], got[3]}, true
			}
		}
		if !ok || box == [4]float64{} {
			return nil, fmt.Errorf("terrain.shade needs the view to shade, " +
				"as south, north, west, east")
		}
		// Said out loud while it happens.
		//
		// Shading a view is a tile fetch and a pass over every cell in it, and
		// it took a minute and a half on a view of Fife with nothing on screen
		// saying so. A layer switched on that does nothing visible for ninety
		// seconds is a layer that does not work, which is what it was reported
		// as.
		go func() {
			ctx := context.Background()
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "shade", What: "shading the terrain in this view"})
			sh, err := s.Hillshade(box[0], box[1], box[2], box[3])
			_, _ = st.Do(ctx, "job.done", "shade")
			if err != nil || sh == nil {
				_, _ = st.Do(ctx, "terrain.shade_failed", nil)
				return
			}
			_, _ = st.Do(ctx, "terrain.shade_set", sh)
		}()
		return map[string]any{"shading": true}, nil
	})

	st.HandleInternalSpec("terrain.shade_set", state.Spec{
		What: "hold the finished hillshade, and warn where most of the view was " +
			"blank ground rather than flat ground",
		Answers: "Answers nothing: the shading goes into the snapshot the " +
			"renderer reads.",
	}, func(w *state.World, p any) (any, error) {
		sh, ok := p.(*state.Coverage)
		if !ok {
			return nil, session.WrongCallback("terrain.shade_set")
		}
		w.Shade = sh
		if sh != nil && sh.NoDataCells > sh.Cells/2 {
			w.Say("terrain shading: most of this view has no elevation data cached")
		}
		return nil, nil
	})

	st.HandleInternalSpec("terrain.shade_failed", state.Spec{
		What: "drop the hillshade and say the view has no elevation cached, so " +
			"a layer that switched on and drew nothing carries its reason",
		Answers: "Answers nothing.",
	}, func(w *state.World, _ any) (any, error) {
		w.Shade = nil
		w.Say("terrain shading: no elevation data for this view")
		return nil, nil
	})

	st.HandleSpec("waterfall.capture", state.Spec{
		What: "take one 200 ms window of what a node's receiver hears and turn " +
			"it into a spectrogram, of the instant it was asked for rather than " +
			"of whenever a worker got round to it",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Primary: true,
				What: "the node to listen at; absent falls back to whichever " +
					"node is selected, and a name this network has not got is " +
					"refused rather than read as none"},
		},
		Returns: []string{"captured"},
		Answers: "`captured` is false without an error whenever there was " +
			"nothing to draw: no engine, no node selected, or nothing on the " +
			"air in that instant. Which of the three it was is said on the " +
			"status line rather than returned.",
		Example: &state.Example{
			Params:   "West Lomond",
			What:     "look at what one node hears during a flood",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
		at := -1
		if name := session.SoleString(p); name != "" {
			for i := range w.Nodes {
				if w.Nodes[i].Name == name {
					at = i
				}
			}
			// Named and not found is a different fact from nothing selected,
			// and the message below can only say the second of the two.
			if at < 0 {
				return nil, session.UnknownNames("waterfall.capture", w.Nodes,
					[]string{name})
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
		img, note := s.Capture(context.Background(), at)
		w.Waterfall, w.WaterfallNote = img, note
		// Silence was the whole fault here. It answered "captured: false" and
		// said nothing at all, so picking it from a menu looked exactly like
		// picking an entry that is not wired up.
		switch {
		case note != "":
			w.Say(note)
		case img == nil && at < 0:
			w.Say("a waterfall is what one node hears: select one, then capture")
		case img == nil:
			w.Say("nothing was on the air in that instant - capture while " +
				"something is transmitting")
		default:
			w.Say("captured 200 ms of what " + w.Nodes[at].Name + " hears")
		}
		return map[string]any{"captured": img != nil}, nil
	})

	st.HandleSpec("coverage.clear", state.Spec{
		What: "take the raster off the map without recomputing anything, for " +
			"when it is covering the ground somebody wants to look at",
		Answers: "Answers nothing at all: there is no state left to report.",
		Example: &state.Example{
			Params: map[string]any{}, What: "put the map back",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
		w.Coverage = nil
		return nil, nil
	})

	st.HandleInternalSpec("coverage.failed", state.Spec{
		What: "say on the status line why a raster job gave up, because a job " +
			"that ends with nothing drawn is otherwise indistinguishable from " +
			"one still running",
		Answers: "Answers nothing.",
	}, func(w *state.World, p any) (any, error) {
		msg := session.SoleString(p)
		w.Say("coverage failed: " + msg)
		return nil, nil
	})
}
