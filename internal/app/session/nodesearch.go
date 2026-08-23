// Finding a node when you cannot type its name.
//
// Imported meshes are named by the people who run them, and on ScotMesh that
// means emoji, box-drawing characters and Gaelic accents: "🏔️ West Lomond 📡"
// is one node's actual name. A script cannot paste that and a person cannot
// reliably type it, so every caller was reduced to fetching all 676 rows and
// doing its own substring match - three of them, three different answers, and
// the ranking is the part that decides which node you end up talking to.
//
// It is one verb rather than a helper in each client because both clients ask
// the same question and a "top result" that differs between them is worse than
// no ranking at all.
package session

import (
	"sort"
	"strings"
	"unicode"

	"github.com/MeshBench/meshbench/internal/app/state"
	"golang.org/x/text/unicode/norm"
)

// searchFloor is the score below which a name is not offered at all. A search
// that returns everything faintly is a search that cannot be trusted to have a
// top result, which is the one thing callers want from it.
const searchFloor = 0.2

// defaultSearchLimit is what comes back when nobody says. Enough to see that
// the top answer beat something, which is what tells you it was a real match.
const defaultSearchLimit = 10

func registerNodeSearch(st *state.Store) {
	st.Handle("nodes.search", func(w *state.World, p any) (any, error) {
		q, _ := stringField(p, "query")
		needle := searchKey(q)
		if needle == "" {
			// Refused rather than answered with everything: an empty query is
			// a bug in the caller, and the whole list is the least useful
			// possible reply to it.
			return nil, badParams("nodes.search needs a query with letters or digits in it")
		}
		limit := defaultSearchLimit
		if v, ok := numField(p, "limit"); ok && v > 0 {
			limit = int(v)
		}

		type scored struct {
			node  state.Node
			score float64
		}
		var hits []scored
		for _, n := range w.Nodes {
			if sc := searchScore(needle, searchKey(n.Name)); sc >= searchFloor {
				hits = append(hits, scored{n, sc})
			}
		}
		// Name breaks ties, so the same query on the same scenario answers the
		// same way twice. Determinism is a feature here as much as anywhere.
		sort.SliceStable(hits, func(i, j int) bool {
			if hits[i].score != hits[j].score {
				return hits[i].score > hits[j].score
			}
			return hits[i].node.Name < hits[j].node.Name
		})
		total := len(hits)
		if len(hits) > limit {
			hits = hits[:limit]
		}

		matches := make([]map[string]any, 0, len(hits))
		for _, h := range hits {
			matches = append(matches, map[string]any{
				"name":  h.node.Name,
				"score": h.score,
				"kind":  h.node.Kind,
				"lat":   h.node.Lat,
				"lon":   h.node.Lon,
			})
		}
		return map[string]any{
			"query": q, "matches": matches, "total": total,
		}, nil
	})
}

// searchKey reduces a name to what somebody would actually type at it.
//
// Decomposed so an accent becomes a separate mark and can be dropped, which is
// what makes "beinn ard" find "Beinn Àrd"; then everything that is not a letter,
// digit or space goes, which is what removes the emoji without needing to know
// which ones. Runs of space collapse, because "West  Lomond" and "West Lomond"
// are the same name to everybody except a comparison.
func searchKey(s string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
			// A combining mark: the accent we just separated off.
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsSpace(r) || r == '-' || r == '_':
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// searchScore rates one normalised name against one normalised query.
//
// The bands are deliberately coarse and ordered by how sure they are, with
// length only ever breaking ties inside a band. A shorter name matching the
// same way is the better answer - "West Lomond" beats "West Lomond Relay Two"
// for the query "west lomond" - but no amount of shortness promotes a name
// from a weaker band to a stronger one.
func searchScore(needle, name string) float64 {
	if name == "" {
		return 0
	}
	switch {
	case name == needle:
		return 1
	case strings.HasPrefix(name, needle):
		return 0.9 + 0.09*tightness(needle, name)
	case strings.Contains(name, needle):
		return 0.8 + 0.09*tightness(needle, name)
	}

	want := strings.Fields(needle)
	got := strings.Fields(name)
	whole, partial := 0, 0
	for _, t := range want {
		switch {
		case containsExact(got, t):
			whole++
		case strings.Contains(name, t):
			partial++
		}
	}
	if whole == len(want) {
		return 0.7 + 0.09*tightness(needle, name)
	}
	if whole+partial == len(want) {
		return 0.6 + 0.09*tightness(needle, name)
	}
	// Some of what was asked for, and the score says how much. Below the floor
	// this is not offered at all.
	found := float64(whole) + 0.5*float64(partial)
	return 0.5 * found / float64(len(want))
}

// tightness is how much of the name the query accounts for: 1 when they are
// the same length, tending to 0 as the name grows around it.
func tightness(needle, name string) float64 {
	return float64(len(needle)) / float64(len(name))
}

func containsExact(tokens []string, want string) bool {
	for _, t := range tokens {
		if t == want {
			return true
		}
	}
	return false
}
