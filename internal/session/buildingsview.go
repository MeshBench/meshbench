// The map's view into the loaded environment.
//
// A read-side bridge, not a verb: footprints are drawn per viewport per
// frame-ish, and the world snapshot is the wrong vehicle for a city of
// polygons. The tile store locks itself, so concurrent reads from the frame
// loop are safe.
package session

import (
	"github.com/MeshBench/meshbench/internal/environ"
	"github.com/MeshBench/meshbench/internal/gui/state"
)

// BuildingsIn is the map's buildings provider: everything standing inside
// the box, as drawable rings, or nil when no environment is loaded.
func (s *Sim) BuildingsIn(south, west, north, east float64) []state.BuildingPoly {
	var env environ.Provider
	if s.eng != nil && s.eng.Env != nil {
		env = s.eng.Env
	} else if s.envDir != "" {
		if s.envView == nil {
			s.envView = environ.OpenTiles(s.envDir)
		}
		env = s.envView
	}
	if env == nil {
		return nil
	}
	bs := env.Buildings(south, west, north, east)
	out := make([]state.BuildingPoly, 0, len(bs))
	for _, b := range bs {
		out = append(out, state.BuildingPoly{Ring: b.Footprint, Material: b.Material})
	}
	return out
}
