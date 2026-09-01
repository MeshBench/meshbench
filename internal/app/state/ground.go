// What ground a study actually stood on.
//
// The model's optimism is only usable because it is acknowledged: a result is
// read as a best case, and the caveat line in the chrome says which kindnesses
// are switched on. Terrain is the one that can go missing without anybody
// choosing it - a refused download, a fetch that failed, a question nobody has
// answered yet - and a profile with no elevation under it is free space, which
// is a bigger claim than any of the caveats already listed.
package state

import "fmt"

// The three states of the ground under a study. Three rather than a bool,
// because the middle one is the one that flatters quietly: a raster that
// walked half a country of real ridges and half a country of sea level is not
// obviously incomplete, and reads as an answer.
const (
	GroundTerrain = "terrain"
	GroundPartial = "partial"
	GroundBare    = "bare-earth"
)

// Ground is the elevation data a study had under it, and whether having none
// of it was a decision somebody made.
type Ground struct {
	// State is GroundTerrain, GroundPartial or GroundBare.
	State string
	// Chosen says somebody has answered the terrain question, either way. It
	// is the difference this type exists for: a bare-earth run an operator
	// asked for is a legitimate offline result, and a bare-earth run nobody was
	// asked about is the model being quietly more optimistic than its own
	// documented best case.
	Chosen bool
	// Note is the honesty line, empty only when the ground is all here.
	Note string
	// Sampled and Cached are the tiles looked for and the tiles found. A
	// sample rather than a census: the count is there to be compared with
	// itself, not quoted as a size.
	Sampled, Cached int
}

// Bare reports a study with no elevation under it anywhere.
func (g Ground) Bare() bool { return g.State == GroundBare }

// Known reports that this ground has been looked at. The zero Ground is a
// session nothing has asked yet, which is not the same as bare.
func (g Ground) Known() bool { return g.State != "" }

// Map is the wire form, for the verbs that carry their own ground in their
// result. Every study answers with the same keys, so a script checks one
// thing however it asked the question.
func (g Ground) Map() map[string]any {
	return map[string]any{
		"state": g.State, "chosen": g.Chosen, "note": g.Note,
		"tiles_sampled": g.Sampled, "tiles_cached": g.Cached,
	}
}

// Caveat is the short form for the chrome's best-case line, which lists what
// the model is being kind about rather than explaining each one.
func (g Ground) Caveat() string {
	switch g.State {
	case GroundBare:
		return "NO TERRAIN"
	case GroundPartial:
		return fmt.Sprintf("terrain only %d%% cached", g.Cached*100/max(1, g.Sampled))
	default:
		return ""
	}
}
