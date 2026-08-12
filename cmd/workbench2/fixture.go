// Loading a shipped network into the state layer.
//
// Through internal/fixture rather than a JSON struct of its own. The ad hoc
// parser this replaced read the six fields the map happened to need, which
// meant the boundaries, the study margin, the antennas and the radio settings
// were all absent from a file that contained every one of them - and absent
// silently, which is the part that matters.
package main

import (
	"github.com/A13xB0/meshcoresim/internal/fixture"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// loaded is everything one fixture supplies the interface.
type loaded struct {
	nodes  []state.Node
	scene  []scenario.Node
	areas  []state.Area
	margin float64
}

func loadFixture(path string) (loaded, error) {
	f, err := fixture.Load(path)
	if err != nil {
		return loaded{}, err
	}
	out := loaded{
		scene:  f.Nodes,
		margin: f.MarginKm,
		nodes:  make([]state.Node, 0, len(f.Nodes)),
	}
	for i, n := range f.Nodes {
		out.nodes = append(out.nodes, state.Node{
			Name: n.Name, Kind: string(n.Kind),
			Lat: n.Position.Lat, Lon: n.Position.Lon,
			HeightM: n.HeightAGLm, TxDBm: n.TxPowerDBm,
			Regions: n.Regions, Firmware: n.Firmware.Version,
			Selected: i == 0,
		})
	}
	for _, a := range f.Areas {
		area := state.Area{Name: a.Name}
		for _, b := range a.Boundaries {
			for _, ring := range b.Rings {
				area.Rings = append(area.Rings, ringOf(ring))
			}
			for _, hole := range b.Holes {
				area.Holes = append(area.Holes, ringOf(hole))
			}
		}
		out.areas = append(out.areas, area)
	}
	return out, nil
}

// ringOf converts a scenario ring to the flat form the renderer wants. The
// interface does not want scenario types in its snapshot: a snapshot the
// renderer holds for several frames must not alias anything the store can
// still write to.
func ringOf(r scenario.Ring) []state.Point {
	out := make([]state.Point, 0, len(r))
	for _, p := range r {
		out = append(out, state.Point{Lat: p.Lat, Lon: p.Lon})
	}
	return out
}
