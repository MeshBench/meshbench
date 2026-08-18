// What a click on the map, or a control in a node window, actually does.
//
// Every one of these goes through a verb rather than writing to the world
// directly. A pointer gesture is not allowed to be a second way of changing
// the network: the map decides what was meant, the store decides what happens,
// and a script driving the same verbs gets the same result.
package workbench

import (
	"context"
	"os"
	"path/filepath"

	"github.com/MeshBench/meshbench/internal/basemap"
	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/session"
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
	c.wbUI.onCommand = func(node, line string) {
		go func() {
			_, _ = c.st.Do(c.ctx, "console.type",
				map[string]any{"node": node, "command": line})
		}()
	}
	c.wbUI.onAction = func(action, node string) {
		go func() {
			if _, err := c.st.Do(c.ctx, action, node); err != nil {
				_, _ = c.st.Do(c.ctx, "ui.said", err.Error())
			}
		}()
	}
	c.wbUI.onCLI = func(node, line string) {
		go func() {
			if _, err := c.st.Do(c.ctx, "console.cli",
				map[string]any{"node": node, "command": line}); err != nil {
				_, _ = c.st.Do(c.ctx, "ui.said", err.Error())
			}
		}()
	}
	// The companion client's actions, which carry more than a node name.
	c.wbUI.onDo = func(verb string, params any) {
		go func() {
			if _, err := c.st.Do(c.ctx, verb, params); err != nil {
				_, _ = c.st.Do(c.ctx, "ui.said", verb+": "+err.Error())
			}
		}()
	}
	c.wbUI.onServe = func(node, kind string) {
		go func() {
			if _, err := c.st.Do(c.ctx, "bench.serve",
				map[string]any{"node": node, "kind": kind}); err != nil {
				_, _ = c.st.Do(c.ctx, "ui.said", "serve: "+err.Error())
			}
		}()
	}
	c.wbUI.onOpenPacket = c.openPacket
	c.sm.SetUI(c.wbUI)
	// The tile cache the old workbench already filled: 37 MB of it on this
	// machine, and the same store, so nothing is downloaded twice. The layer
	// is the remembered choice, like the old workbench's.
	if cache, err := os.UserCacheDir(); err == nil {
		layerID := "carto-dark"
		if id := c.sm.Basemap(); id != "" {
			layerID = id
		}
		c.mv.Tiles = comp.NewTiles(filepath.Join(cache, "meshcoresim", "tiles"), layerID)
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
		go func() {
			_, _ = c.st.Do(c.ctx, verb, names)
			// The budget follows the selection: it is a panel about whatever
			// is selected, and asking for it separately would let the two
			// disagree about what that is.
			_, _ = c.st.Do(c.ctx, "budget.for_selection", nil)
		}()
	}
	c.mv.OnMove = func(name string, lat, lon float64) {
		go func() {
			_, _ = c.st.Do(c.ctx, "nodes.move",
				map[string]any{"name": name, "lat": lat, "lon": lon})
		}()
	}
}
