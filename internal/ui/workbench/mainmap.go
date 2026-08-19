// The map's tools wired to the session: what a click on the map means.
// Split from Run, which had grown past the file limit wiring everything.
package workbench

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/shell"
)

// wireMapTools connects the map's gestures to verbs.
func wireMapTools(mv *comp.MapView, mapTop *mapTools, sh *shell.Shell,
	st *state.Store, ctx context.Context) {
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
	// The link tool asks about exactly the pair it was given - link.pair,
	// which answers whether or not the engine is warm and whatever the
	// margin is. It used to route through budget.for_selection, which kept
	// only the first node's strongest link: the chosen far end was never
	// consulted, and a far-apart pair produced an empty panel.
	mv.OnLinkPair = func(a, b comp.LinkEnd) {
		mv.PinnedLink = true
		// The answer appears where the operator is looking: completing a
		// pair opens the Link panel in its own window, unless it is already
		// on screen - in this view's layout or in a window of its own.
		if sh != nil && !linkPanelVisible(sh) && sh.OnPopOut != nil {
			sh.OnPopOut("Link")
		}
		params := map[string]any{"a": linkEndParam(a), "b": linkEndParam(b)}
		var sel []string
		for _, e := range []comp.LinkEnd{a, b} {
			if e.Node != "" {
				sel = append(sel, e.Node)
			}
		}
		from, to := linkEndName(a), linkEndName(b)
		go func() {
			if len(sel) > 0 {
				_, _ = st.Do(ctx, "nodes.select_many", sel)
			}
			if _, err := st.Do(ctx, "link.pair", params); err != nil {
				_, _ = st.Do(ctx, "ui.said", "link: "+err.Error())
				return
			}
			_, _ = st.Do(ctx, "ui.said",
				from+" to "+to+" is pinned in the Link panel - Esc releases it")
		}()
	}
	// A half-made pick is a mode, and a mode must say so.
	mv.OnLinkArmed = func(end comp.LinkEnd) {
		hint := "link: first end " + linkEndName(end) +
			" - click the far end, a node or bare ground; Esc cancels"
		go func() { _, _ = st.Do(ctx, "ui.said", hint) }()
	}
	mv.OnLinkCancel = func() {
		go func() { _, _ = st.Do(ctx, "ui.said", "link released") }()
	}
}

// linkPanelVisible reports whether the Link panel is already on screen:
// docked in the current view's arrangement, or popped into its own window.
func linkPanelVisible(sh *shell.Shell) bool {
	if sh.PoppedOut != nil && sh.PoppedOut("Link") {
		return true
	}
	for _, n := range shell.PanelsIn(sh.View) {
		if n == "Link" {
			return true
		}
	}
	return false
}

// linkEndParam is what link.pair is told about one end.
func linkEndParam(e comp.LinkEnd) any {
	if e.Node != "" {
		return e.Node
	}
	return map[string]any{"lat": e.Lat, "lon": e.Lon}
}

// linkEndName is how an end reads in the status bar.
func linkEndName(e comp.LinkEnd) string {
	if e.Node != "" {
		return e.Node
	}
	return fmt.Sprintf("%.4f, %.4f", e.Lat, e.Lon)
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
