// What a click on the map, or a control in a node window, actually does.
//
// Every one of these goes through a verb rather than writing to the world
// directly. A pointer gesture is not allowed to be a second way of changing
// the network: the map decides what was meant, the store decides what happens,
// and a script driving the same verbs gets the same result.
package workbench

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/world/basemap"
)

// callbacks is what the map and node windows reach to do their work.
type callbacks struct {
	wbUI       *workbenchUI
	mv         *comp.MapView
	st         *state.Store
	ctx        context.Context
	sm         *session.Sim
	openPacket func(id uint64)
	// chooser posts a list to pick from; do runs a verb and reports failure.
	chooser func(string, []string, func(string))
	do      Do
}

// wire attaches every handler.
func (c callbacks) wire() {
	c.wbUI.OnCommand = func(node, line string) {
		go func() {
			_, _ = c.st.Do(c.ctx, "console.type",
				map[string]any{"node": node, "command": line})
		}()
	}
	c.wbUI.OnAction = func(action, node string) {
		go func() {
			if _, err := c.st.Do(c.ctx, action, node); err != nil {
				_, _ = c.st.Do(c.ctx, "ui.said", err.Error())
			}
		}()
	}
	c.wbUI.OnCLI = func(node, line string) {
		go func() {
			if _, err := c.st.Do(c.ctx, "console.cli",
				map[string]any{"node": node, "command": line}); err != nil {
				_, _ = c.st.Do(c.ctx, "ui.said", err.Error())
			}
		}()
	}
	// The companion client's actions, which carry more than a node name.
	c.wbUI.OnDo = func(verb string, params any) {
		go func() {
			if _, err := c.st.Do(c.ctx, verb, params); err != nil {
				_, _ = c.st.Do(c.ctx, "ui.said", verb+": "+err.Error())
			}
		}()
	}
	c.wbUI.OnServe = func(node, kind string) {
		go func() {
			if _, err := c.st.Do(c.ctx, "bench.serve",
				map[string]any{"node": node, "kind": kind}); err != nil {
				_, _ = c.st.Do(c.ctx, "ui.said", "serve: "+err.Error())
			}
		}()
	}
	c.wbUI.OnOpenPacket = c.openPacket
	c.sm.SetUI(c.wbUI)
	// The tile cache the old workbench already filled: 37 MB of it on this
	// machine, and the same store, so nothing is downloaded twice. The layer
	// is the remembered choice, like the old workbench's.
	if cache, err := os.UserCacheDir(); err == nil {
		// OpenStreetMap by default, because it needs no key: the CARTO layers
		// return a "API KEY REQUIRED" tile without a token, so a first run that
		// defaulted to one opened onto a wall of watermarks. With a key in the
		// environment the CARTO dark map is the better ground for data, which
		// is what it was designed to be. A remembered choice still wins.
		layerID := "osm"
		if basemap.CartoKey() != "" {
			layerID = "carto-dark"
		}
		if id := c.sm.Basemap(); id != "" {
			layerID = id
		}
		c.mv.Tiles = comp.NewTiles(filepath.Join(cache, "meshbench", "tiles"), layerID)
		c.mv.OverlayTiles = nil
		for _, ov := range basemap.OverlaysFor(layerID) {
			c.mv.OverlayTiles = append(c.mv.OverlayTiles,
				comp.NewTiles(filepath.Join(cache, "meshbench", "tiles"), ov.ID))
		}
	}
	// The basemap picker at the top of the map's layer panel: base layers
	// only - overlays are not a map - chosen through the shell's chooser and
	// remembered across launches.
	c.mv.OnBasemap = func() {
		var names []string
		for _, l := range basemap.Layers() {
			if l.Kind == basemap.Base {
				names = append(names, l.Name)
			}
		}
		c.chooser("Basemap", names, func(picked string) {
			for _, l := range basemap.Layers() {
				if l.Name == picked && c.mv.Tiles != nil {
					c.mv.Tiles.SetLayer(l.ID)
					// The overlays follow their base, or leave with it.
					c.mv.OverlayTiles = nil
					if cache, err := os.UserCacheDir(); err == nil {
						for _, ov := range basemap.OverlaysFor(l.ID) {
							c.mv.OverlayTiles = append(c.mv.OverlayTiles,
								comp.NewTiles(filepath.Join(cache, "meshbench", "tiles"), ov.ID))
						}
					}
					c.do("map.basemap", map[string]any{"id": l.ID})
				}
			}
		})
	}
	// The map decides, the store changes. A pointer gesture is not allowed to
	// write to the world directly, so both of these go through the same verbs
	// a script would use.
	c.mv.OnSelect = func(names []string, additive bool) {
		verb := "nodes.select_many"
		if additive {
			verb = "nodes.add_to_selection"
		}
		// While a pair is pinned, selecting still selects - only the budget
		// refresh is held back, because it would overwrite the pinned pair
		// with the new selection's strongest link on the very next click.
		pinned := c.mv.PinnedLink
		go func() {
			_, _ = c.st.Do(c.ctx, verb, names)
			if pinned {
				return
			}
			// The budget follows the selection: it is a panel about whatever
			// is selected, and asking for it separately would let the two
			// disagree about what that is.
			_, _ = c.st.Do(c.ctx, "budget.for_selection", nil)
		}()
	}
	c.mv.OnDelete = c.deleteNodes
	c.mv.OnMove = func(name string, lat, lon float64) {
		go func() {
			_, _ = c.st.Do(c.ctx, "nodes.move",
				map[string]any{"node": name, "lat": lat, "lon": lon})
		}()
	}
}

// deleteNodes asks first, then removes them.
//
// Asked because it is the one gesture on the map that cannot be undone: a node
// carries its regions, its firmware and its position, and the way back is to
// place it again and set all three. The question goes through the shell's
// chooser, which is the single way anything here picks from a list.
func (c callbacks) deleteNodes(names []string) {
	if len(names) == 0 || c.chooser == nil {
		return
	}
	title := "Delete " + names[0] + "?"
	if len(names) > 1 {
		title = fmt.Sprintf("Delete these %d nodes?", len(names))
	}
	c.chooser(title, []string{"Delete", "Keep"}, func(picked string) {
		if picked != "Delete" {
			return
		}
		// One verb call per node, in order, on a worker. nodes.delete
		// rebuilds the seeded scenario and re-warms the link matrix, and a
		// warm cancels the one before it - so trimming several nodes by hand
		// costs one measurement of the matrix rather than one per node.
		go func() {
			for _, name := range names {
				if _, err := c.st.Do(c.ctx, "nodes.delete",
					map[string]any{"node": name}); err != nil {
					_, _ = c.st.Do(c.ctx, "ui.said", "delete: "+err.Error())
					return
				}
			}
		}()
	})
}
