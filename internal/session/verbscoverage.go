// Coverage, terrain shading and the waterfall: the three things the map can
// be asked to draw on top of itself, and the three that take long enough to
// need a job and a failure verb each.
package session

import (
	"context"
	"fmt"
	"math"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

func registerCoverageVerbs(st *state.Store, s *Sim) {
	st.Handle("coverage.compute", func(w *state.World, p any) (any, error) {
		// The selected node unless told otherwise, because "coverage from
		// here" is what somebody means when they have just clicked a node.
		at := -1
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
		if at < 0 || at >= len(s.nodes) {
			return nil, fmt.Errorf("no node selected to compute coverage from")
		}
		n := s.nodes[at]
		// The whole-map job with a station list of one: the GPU fold, the
		// buildings, the resolution knob and the percentage all arrive for
		// free, where the 160-cell tile walk this replaced had none of
		// them. The box is the node's 60 km study square, as it always was.
		dLat := 60.0 / 111.32
		dLon := 60.0 / (111.32 * math.Cos(n.Position.Lat*math.Pi/180))
		// A floor under the resolution knob: 800 cells is ~150 m over the
		// study box, which is what "coverage from here" is for - the knob
		// can still push it higher for everything at once.
		cells := s.coverageCells()
		if cells < 800 {
			cells = 800
		}
		return s.startCoverageMap(st, w, map[string]any{
			"station": n.Name, "cells": float64(cells),
			"south": n.Position.Lat - dLat, "north": n.Position.Lat + dLat,
			"west": n.Position.Lon - dLon, "east": n.Position.Lon + dLon,
		})
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
			sh, err := s.hillshade(box[0], box[1], box[2], box[3])
			_, _ = st.Do(ctx, "job.done", "shade")
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
		// Captured here rather than on a worker: this is one 200 ms window of
		// samples through one FFT, not a national raster, and doing it inline
		// means the capture is of the instant that was asked for rather than
		// of whenever a goroutine got round to it.
		img, note := s.capture(context.Background(), at)
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

	st.Handle("coverage.clear", func(w *state.World, _ any) (any, error) {
		w.Coverage = nil
		return nil, nil
	})

	st.Handle("coverage.failed", func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		w.Say("coverage failed: " + msg)
		return nil, nil
	})
}
