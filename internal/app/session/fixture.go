// Loading a shipped network into the state layer.
//
// Through internal/fixture rather than a JSON struct of its own. The ad hoc
// parser this replaced read the six fields the map happened to need, which
// meant the boundaries, the study margin, the antennas and the radio settings
// were all absent from a file that contained every one of them - and absent
// silently, which is the part that matters.
package session

import (
	"github.com/MeshBench/meshbench/internal/app/fixture"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Loaded is everything one fixture supplies the interface.
type Loaded struct {
	nodes      []state.Node
	scene      []scenario.Node
	areas      []state.Area
	margin     float64
	sends      []state.Send
	assertions []state.Assertion
}

func LoadFixture(path string) (Loaded, error) {
	f, err := fixture.Load(path)
	if err != nil {
		return Loaded{}, err
	}
	out := Loaded{
		scene:  f.Nodes,
		margin: f.MarginKm,
		nodes:  stateNodes(f.Nodes),
	}
	for _, snd := range f.Sends {
		out.sends = append(out.sends, state.Send{
			Node: snd.Node, AtMs: snd.AtMs, EveryMs: snd.EveryMs,
			Command: snd.Command,
		})
	}
	for _, a := range f.Assertions {
		out.assertions = append(out.assertions, state.Assertion{
			Kind: a.Kind, Node: a.Node, WithinMs: a.WithinMs,
			AtLeast: a.AtLeast, AtMost: a.AtMost, MaxPct: a.MaxPct,
		})
	}
	for _, a := range f.Areas {
		out.areas = append(out.areas, areaOf(a.Name, a.Boundaries))
	}
	return out, nil
}

// areaOf is the one way a set of boundaries becomes a study area.
//
// One, because there were three, and two of them dropped the holes: a loch or
// an enclave came out of the GeoJSON reader, survived into the scenario, and
// then vanished on its way to the snapshot the map draws from - so a study area
// accepted by name covered water that the same area loaded from a fixture did
// not. Nothing said so; the shape was simply a little larger than the place.
func areaOf(name string, boundaries []scenario.Boundary) state.Area {
	area := state.Area{Name: name}
	for _, b := range boundaries {
		for _, ring := range b.Rings {
			area.Rings = append(area.Rings, ringOf(ring))
		}
		for _, hole := range b.Holes {
			area.Holes = append(area.Holes, ringOf(hole))
		}
	}
	return area
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

// patternSamples is how finely the antenna pattern is sampled: every ten
// degrees, which is smooth enough at the size it is drawn and cheap enough to
// keep on every node in the snapshot.
const patternSamples = 36

// patternOf samples an antenna's horizontal gain, or returns nil.
//
// At the horizon, because a map is a plan view: what a downtilt does to the
// gain towards a hilltop is a question for the budget panel, which has the
// geometry to answer it.
func patternOf(n scenario.Node) []float64 {
	if n.Antenna.Pattern == nil {
		return nil
	}
	out := make([]float64, patternSamples)
	for i := range out {
		out[i] = n.Antenna.GainTowardsDBi(float64(i)*360/patternSamples, 0)
	}
	return out
}

// antennaOf is what a node's antenna is, in the words it was chosen in, for the
// form that edits it.
//
// A pattern this package cannot describe leaves the type empty, which the panel
// says out loud. Guessing at a name would put a control on screen offering to
// change an antenna into something it never was.
func antennaOf(n scenario.Node) state.Antenna {
	if n.Antenna.Pattern == nil {
		return state.Antenna{}
	}
	shape, err := antenna.ShapeOf(n.Antenna.Pattern)
	if err != nil {
		return state.Antenna{}
	}
	return state.Antenna{
		Type:          shape.Type,
		GainDBiPeak:   shape.GainDBiPeak,
		BeamwidthDeg:  shape.BeamwidthDeg,
		FrontToBackDB: shape.FrontToBackDB,
		BearingDeg:    n.Antenna.BearingDeg,
		DowntiltDeg:   n.Antenna.DowntiltDeg,
		Polarisation:  string(n.Antenna.Polarisation),
		FeedlineDB:    n.Antenna.FeedlineDB,
	}
}
