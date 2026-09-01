package scenario

import (
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/rf/antenna"
)

// BoardAntenna is the antenna a node gets when nobody has chosen one.
//
// A vertical collinear, from the board's own figures, pointing nowhere in
// particular - which is honest for an omni and would be a lie for a beam, so
// nothing here ever returns one. Whoever wants a beam says so.
//
// The four decibels are the difference between the stub a board ships with and
// the antenna anybody who mounts a repeater on a mast actually buys. It is a
// guess, and it is the same guess for every node so that comparing two of them
// is still meaningful; a scenario that cares sets the gain itself.
//
// One function rather than the same literal at each site, because three copies
// of "what an antenna is by default" is three places for a network built by
// import and a network built by hand to come apart.
func BoardAntenna(profile hw.Board) antenna.Mounted {
	return antenna.Mounted{
		Pattern:      antenna.Collinear{GainDBiPeak: profile.AntennaDBi + 4},
		Polarisation: antenna.Vertical,
		FeedlineDB:   profile.FeedlineDB,
	}
}
