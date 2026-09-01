// What a study stood on.
package meshbench

// The three states of the ground under a study, as the workbench reports them.
const (
	GroundTerrain = "terrain"
	GroundPartial = "partial"
	GroundBare    = "bare-earth"
)

// Ground is what elevation data a study actually had under it, and whether
// having none of it was a decision.
//
// Every study answers with the same keys, so a script checks one thing however
// it asked the question. Chosen is the distinction that matters: a bare-earth
// run the operator asked for is a legitimate offline result, and one nobody was
// asked about is the model being quietly more optimistic than its own
// documented best case.
type Ground struct {
	State        string `json:"state"`
	Chosen       bool   `json:"chosen"`
	Note         string `json:"note"`
	TilesSampled int    `json:"tiles_sampled"`
	TilesCached  int    `json:"tiles_cached"`
}

// Bare reports a study with no elevation under it anywhere, which the
// propagation model prices as free space: the most optimistic answer it has.
func (g Ground) Bare() bool { return g.State == GroundBare }
