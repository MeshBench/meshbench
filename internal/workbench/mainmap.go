// The map's tools wired to the session: what a click on the map means.
// Split from Run, which had grown past the file limit wiring everything.
package workbench

import (
	"context"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
)

// wireMapTools connects the map's gestures to verbs.
func wireMapTools(mv *comp.MapView, mapTop *mapTools, st *state.Store, ctx context.Context) {
	// Rastering the viewport is the map asking about its own borders, at a
	// resolution matched to the screen showing it.
	mv.OnRasterView = func(south, west, north, east float64, cells int) {
		go func() {
			if _, err := st.Do(ctx, "coverage.map", map[string]any{
				"south": south, "west": west, "north": north, "east": east,
				"cells": float64(cells),
			}); err != nil {
				_, _ = st.Do(ctx, "ui.said", err.Error())
			}
		}()
	}
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
}

// rasterMenuIntercept is the menu's raster entries: the rasters exist to
// be looked at, and computing one behind a switched-off layer was a click
// that did nothing.
func rasterMenuIntercept(mv *comp.MapView, st *state.Store, ctx context.Context) func(string) bool {
	return func(action string) bool {
		switch action {
		case "coverage.map":
			mv.Layers.Coverage = true
		case "coverage.viewport", "coverage.selection.viewport":
			mv.Layers.Coverage = true
			south, west, north, east, ok := mv.ViewportBox()
			if !ok {
				return true
			}
			params := map[string]any{
				"south": south, "west": west, "north": north, "east": east,
				"cells": float64(mv.ViewportCells()),
			}
			if action == "coverage.selection.viewport" {
				params["station"] = "selected"
			}
			go func() {
				if _, err := st.Do(ctx, "coverage.map", params); err != nil {
					_, _ = st.Do(ctx, "ui.said", err.Error())
				}
			}()
			return true
		}
		return false
	}
}
