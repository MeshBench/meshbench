// What ground a study is standing on, and whether that was chosen.
//
// A study over ground this machine has not got is not an error: it is a
// perfectly ordinary offline run, and the operator who switched downloads off
// asked for it. What it must never be is quiet. Free space is the most
// optimistic answer the propagation model can produce, so a hill that would
// have blocked a link is simply not there, and the result is a plausible number
// wrong in the flattering direction. This is the one place that decides how
// much of the ground is actually here, so every study says the same thing about
// it in the same words.
package session

import (
	"fmt"
	"math"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// groundSampleEdge is how many points across a study's box are turned into
// tile coordinates and looked for on disk.
//
// A sample rather than a census, because a census of a national box is a
// hundred thousand stat calls and this is asked on the way into every study.
// Sixteen a side is 256 probes at most, which over any box big enough to
// matter lands on tiles spread across the whole of it: the question being
// answered is "is the ground here", not "exactly how many tiles are missing".
const groundSampleEdge = 16

// GroundUnder is the ground under a set of nodes, which is what the
// node-shaped studies stand on.
func (s *Sim) GroundUnder(nodes []scenario.Node) state.Ground {
	if len(nodes) == 0 {
		return state.Ground{}
	}
	south, north := math.Inf(1), math.Inf(-1)
	west, east := math.Inf(1), math.Inf(-1)
	for i := range nodes {
		south = math.Min(south, nodes[i].Position.Lat)
		north = math.Max(north, nodes[i].Position.Lat)
		west = math.Min(west, nodes[i].Position.Lon)
		east = math.Max(east, nodes[i].Position.Lon)
	}
	return s.GroundOver(south, north, west, east)
}

// GroundOver is the ground under a box, which is what a raster stands on.
func (s *Sim) GroundOver(south, north, west, east float64) state.Ground {
	_, asked := s.TerrainConsent()
	ts, ok := s.terrain().(*terrain.TileStore)
	if !ok || ts == nil {
		// No tile cache at all is a machine configured that way rather than one
		// that lost a download, so it counts as chosen - but it is still bare
		// earth and still has to say so.
		g := state.Ground{State: state.GroundBare, Chosen: true}
		g.Note = groundNote(g)
		return g
	}
	return groundFrom(ts.EstimateTiles(groundSampleTiles(south, north, west, east, ts.Zoom)), asked)
}

// groundSampleTiles is the deduplicated tile coordinates under an evenly
// spaced grid of points across a box.
func groundSampleTiles(south, north, west, east float64, zoom int) [][2]int {
	if zoom <= 0 {
		zoom = terrain.DefaultZoom
	}
	seen := map[[2]int]bool{}
	var out [][2]int
	for i := 0; i < groundSampleEdge; i++ {
		for j := 0; j < groundSampleEdge; j++ {
			// The half-step offsets keep every probe inside the box: a grid
			// that starts at the corner asks about the tile beyond the edge,
			// which on a box drawn round a coastline is open sea.
			lat := south + (north-south)*(float64(i)+0.5)/groundSampleEdge
			lon := west + (east-west)*(float64(j)+0.5)/groundSampleEdge
			k := [2]int{}
			k[0], k[1] = terrain.TileXY(lat, lon, zoom)
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

// groundFrom reads a tile estimate as a state, and writes the line that goes
// with it.
func groundFrom(est terrain.Estimate, asked bool) state.Ground {
	g := state.Ground{Chosen: asked, Sampled: est.Tiles, Cached: est.Cached}
	switch {
	case est.Tiles == 0 || est.Cached >= est.Tiles:
		g.State = state.GroundTerrain
		return g
	case est.Cached == 0:
		g.State = state.GroundBare
	default:
		g.State = state.GroundPartial
	}
	g.Note = groundNote(g)
	return g
}

// groundNote is what a study says about the ground it did not have.
//
// It names free space rather than saying "no terrain", because "no terrain"
// reads as a missing decoration and free space is a claim: every ridge in the
// study area is flat, so links close here that do not close in the world.
func groundNote(g state.Ground) string {
	const consequence = "so this is bare earth, which the propagation model " +
		"treats as free space: the most optimistic answer it has, and more " +
		"optimistic than the best case the rest of the model is documented as"
	if g.State == state.GroundPartial {
		return fmt.Sprintf(
			"partial terrain: %d of %d sampled tiles under this study are cached "+
				"and the rest is bare earth, so some of this answer is free space "+
				"and nothing in it says which part", g.Cached, g.Sampled)
	}
	if !g.Chosen {
		return "no terrain: nothing under this study is cached and nobody has been " +
			"asked whether it may be downloaded, " + consequence +
			". terrain.allow settles it either way"
	}
	return "no terrain: nothing under this study is cached and terrain downloads " +
		"are off, " + consequence
}

// NoteGround records what a study stood on and says it aloud.
//
// For the verbs whose purpose is to answer when nothing else will: link.pair
// exists precisely for the moment somebody points at two repeaters before a
// warm and asks why they cannot hear each other, and a refusal there withholds
// the one answer that was wanted. It still never answers quietly.
//
// The caller puts the ground in its own result as well. This puts it where the
// interface reads it and where the log keeps it; a study is read in three
// places and the caveat has to survive all three.
func NoteGround(w *state.World, verb string, g state.Ground) {
	w.Ground = g
	if g.Note != "" {
		w.Say(verb + ": " + g.Note)
	}
}

// StudyGround is NoteGround with the refusal the rasters make: a picture of
// where a network works, drawn over ground nobody chose to do without, is not
// an answer.
//
// Refused in that one case only, because the caller has not decided anything
// yet and a plausible picture is worse than a question. An operator who has
// answered - either way - gets their study and the note that goes with it,
// because an offline run over cached ground is exactly what refusing downloads
// is for.
func StudyGround(w *state.World, verb string, g state.Ground) error {
	NoteGround(w, verb, g)
	if g.Bare() && !g.Chosen {
		return fmt.Errorf("%s: %s", verb, g.Note)
	}
	return nil
}
