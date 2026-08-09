// Package planning answers the questions an operator actually asks: where
// should the next node go, and what would changing this one buy.
//
// Everything here is comparative. The absolute numbers carry the systematic
// optimism written up in docs/shortcomings.md — no multipath, bare-earth
// terrain, an idealised demodulator — and most of that cancels between two
// answers computed the same way. "This mast versus that one" survives the model
// error; "this mast will work" survives it much less well.
package planning

import (
	"fmt"
	"math"
	"sort"

	"github.com/A13xB0/meshcoresim/internal/coverage"
)

// Candidate is a place a node might go.
type Candidate struct {
	Name       string
	Lat, Lon   float64
	HeightAGLm float64
}

// SiteScore is what a candidate would add to the network.
type SiteScore struct {
	Candidate Candidate

	// NewCellsServed is cells the network cannot reach today that this
	// candidate would. It is the number that matters, and it is not the same as
	// the candidate's own coverage: a site with enormous coverage that all
	// overlaps an existing repeater adds nothing.
	NewCellsServed int

	// OwnCellsServed is its total two-way coverage, kept so a site that scores
	// badly can be told apart from one that covers nothing at all.
	OwnCellsServed int

	// RedundancyAdded counts cells that had exactly one server and would gain a
	// second — resilience rather than reach, and worth separating because a
	// network with no gaps can still be one mast away from failing.
	RedundancyAdded int
}

// RankSites scores candidates by what each adds to an existing network.
//
// Greedy and one-at-a-time on purpose: it answers "what is the best next site",
// not "what is the best set of five". Those are different questions and the
// second one is a covering problem whose answer cannot be read off this list —
// the second-best site here is frequently redundant with the best.
func RankSites(existing *coverage.Combined, candidates []*coverage.Raster, names []Candidate) ([]SiteScore, error) {
	if existing == nil {
		return nil, fmt.Errorf("planning: no existing network to compare against")
	}
	if len(candidates) != len(names) {
		return nil, fmt.Errorf("planning: %d rasters for %d candidates", len(candidates), len(names))
	}

	scores := make([]SiteScore, 0, len(candidates))
	for i, r := range candidates {
		if r.Width != existing.Width || r.Height != existing.Height {
			return nil, fmt.Errorf("planning: %s was computed over a different grid", names[i].Name)
		}
		s := SiteScore{Candidate: names[i]}
		for j := range r.Cells {
			if !r.Cells[j].Workable() {
				continue
			}
			s.OwnCellsServed++
			switch existing.ServingCount[j] {
			case 0:
				s.NewCellsServed++
			case 1:
				s.RedundancyAdded++
			}
		}
		scores = append(scores, s)
	}

	sort.SliceStable(scores, func(a, b int) bool {
		if scores[a].NewCellsServed != scores[b].NewCellsServed {
			return scores[a].NewCellsServed > scores[b].NewCellsServed
		}
		// A tie on new reach is broken by resilience, not by raw coverage: two
		// sites that fill the same gap are separated by what else they back up.
		return scores[a].RedundancyAdded > scores[b].RedundancyAdded
	})
	return scores, nil
}

// HeightGain is what another few metres of mast would buy at one site.
type HeightGain struct {
	HeightAGLm  float64
	CellsServed int
	// AddedOverPrevious is the gain from the step below it. Diminishing returns
	// are the point of the table: the useful output is "12 m buys you most of
	// it and 20 m buys almost nothing more", not a single number.
	AddedOverPrevious int
}

// HeightSweep summarises a set of rasters computed at increasing mast heights.
//
// The rasters have to be supplied rather than computed here, because computing
// them needs terrain and a link budget that belong to the caller. This function
// is the part that is easy to get subtly wrong: reporting totals without
// differences invites reading a big number as a big improvement.
func HeightSweep(heights []float64, rasters []*coverage.Raster) ([]HeightGain, error) {
	if len(heights) != len(rasters) {
		return nil, fmt.Errorf("planning: %d heights for %d rasters", len(heights), len(rasters))
	}
	idx := make([]int, len(heights))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return heights[idx[a]] < heights[idx[b]] })

	out := make([]HeightGain, 0, len(heights))
	prev := 0
	for n, i := range idx {
		served := 0
		for _, c := range rasters[i].Cells {
			if c.Workable() {
				served++
			}
		}
		g := HeightGain{HeightAGLm: heights[i], CellsServed: served}
		if n > 0 {
			g.AddedOverPrevious = served - prev
		}
		prev = served
		out = append(out, g)
	}
	return out, nil
}

// KneeHeight is the height beyond which further mast buys little.
//
// Defined as the lowest height whose step gains less than `fraction` of the
// best step seen. Operators ask "how tall does the mast need to be", and the
// honest answer is a point of diminishing returns rather than a maximum —
// the maximum is always "as tall as you can afford", which helps nobody.
func KneeHeight(sweep []HeightGain, fraction float64) (float64, bool) {
	if len(sweep) < 2 {
		return 0, false
	}
	best := 0
	for _, g := range sweep[1:] {
		if g.AddedOverPrevious > best {
			best = g.AddedOverPrevious
		}
	}
	if best <= 0 {
		return sweep[0].HeightAGLm, true
	}
	threshold := float64(best) * fraction
	for i := 1; i < len(sweep); i++ {
		if float64(sweep[i].AddedOverPrevious) < threshold {
			return sweep[i-1].HeightAGLm, true
		}
	}
	return sweep[len(sweep)-1].HeightAGLm, true
}

// LinkMargin summarises one link for a report.
type LinkMargin struct {
	From, To    string
	OutboundDB  float64
	InboundDB   float64
	DistanceKm  float64
	LimitedBy   string // "outbound", "inbound" or "balanced"
	Workable    bool
	OneWayOnly  bool
	WorstCaseDB float64
}

// Summarise turns a computed cell into the sentence an operator needs.
//
// LimitedBy exists because "add 3 dB" is only useful once you know at which
// end. The commonest real answer is that the handheld cannot be heard, and no
// amount of work on the repeater's transmitter changes that.
func Summarise(from, to string, distanceKm float64, c coverage.Cell) LinkMargin {
	l := LinkMargin{
		From: from, To: to,
		OutboundDB: c.OutboundMarginDB, InboundDB: c.InboundMarginDB,
		DistanceKm:  distanceKm,
		Workable:    c.Workable(),
		OneWayOnly:  c.OneWay(),
		WorstCaseDB: math.Min(c.OutboundMarginDB, c.InboundMarginDB),
	}
	switch {
	case math.Abs(c.OutboundMarginDB-c.InboundMarginDB) < 1:
		l.LimitedBy = "balanced"
	case c.InboundMarginDB < c.OutboundMarginDB:
		l.LimitedBy = "inbound"
	default:
		l.LimitedBy = "outbound"
	}
	return l
}
