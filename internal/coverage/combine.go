package coverage

import (
	"errors"
	"math"
)

var (
	errNoRasters    = errors.New("coverage: nothing to combine")
	errGridMismatch = errors.New("coverage: rasters do not share a grid; combining them would map nowhere")
)

// Combined is the network's coverage rather than one node's: for every cell,
// the best link available and how many nodes offer one.
//
// The count is the point. A cell served by one repeater and a cell served by
// four look identical on a coverage map and are completely different networks —
// one of them survives a mast coming down.
type Combined struct {
	Raster

	// BestMarginDB is the strongest *two-way* margin any node offers. Cells
	// with no two-way link keep the best one-way margin they have, which is
	// negative by construction, so the scale stays continuous.
	BestMarginDB []float64

	// BestNode indexes into the nodes passed to Combine; -1 where nothing
	// reaches.
	BestNode []int

	// ServingCount is how many nodes give a workable two-way link.
	ServingCount []int
}

// Combine merges per-node rasters.
//
// All rasters must share a grid. That is checked rather than assumed: merging
// two rasters over different boxes produces a plausible map of nowhere.
func Combine(rasters []*Raster) (*Combined, error) {
	if len(rasters) == 0 {
		return nil, errNoRasters
	}
	first := rasters[0]
	for _, r := range rasters[1:] {
		if r.Width != first.Width || r.Height != first.Height ||
			r.South != first.South || r.North != first.North ||
			r.West != first.West || r.East != first.East {
			return nil, errGridMismatch
		}
	}

	n := first.Width * first.Height
	c := &Combined{
		Raster:       Raster{South: first.South, North: first.North, West: first.West, East: first.East, Width: first.Width, Height: first.Height, FreqMHz: first.FreqMHz},
		BestMarginDB: make([]float64, n),
		BestNode:     make([]int, n),
		ServingCount: make([]int, n),
	}
	c.Cells = make([]Cell, n)

	for i := 0; i < n; i++ {
		c.BestMarginDB[i] = math.Inf(-1)
		c.BestNode[i] = -1
		allNoData := true

		for ni, r := range rasters {
			cell := r.Cells[i]
			if cell.NoData {
				continue
			}
			allNoData = false
			if cell.Workable() {
				c.ServingCount[i]++
			}
			// The margin that describes a link is the weaker direction. Taking
			// the better one would call a link workable on the strength of the
			// half that was never in doubt.
			m := math.Min(cell.OutboundMarginDB, cell.InboundMarginDB)
			if m > c.BestMarginDB[i] {
				c.BestMarginDB[i] = m
				c.BestNode[i] = ni
				c.Cells[i] = cell
			}
		}
		if allNoData {
			c.Cells[i] = Cell{NoData: true}
			c.BestMarginDB[i] = math.NaN()
		}
	}
	return c, nil
}

// GapCells counts cells with no two-way service, ignoring cells with no data.
// Returned with the total so a percentage means something: a raster that is
// mostly sea should not report 90% gaps.
func (c *Combined) GapCells() (gaps, known int) {
	for i, cell := range c.Cells {
		if cell.NoData {
			continue
		}
		known++
		if c.ServingCount[i] == 0 {
			gaps++
		}
	}
	return gaps, known
}

// SinglePointOfFailure counts cells served by exactly one node — the cells that
// go dark when that node does.
func (c *Combined) SinglePointOfFailure() int {
	n := 0
	for _, s := range c.ServingCount {
		if s == 1 {
			n++
		}
	}
	return n
}

// Redundancy is the mean number of serving nodes over cells that have any
// service. Averaging over unserved cells too would let a large empty area drag
// the figure down and make a well-meshed core look fragile.
func (c *Combined) Redundancy() float64 {
	var sum, count float64
	for i, cell := range c.Cells {
		if cell.NoData || c.ServingCount[i] == 0 {
			continue
		}
		sum += float64(c.ServingCount[i])
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / count
}

// Fold accumulates stations into a Combined one raster at a time - the
// whole-map GPU path prices hundreds of stations, and holding every raster
// to Combine them at the end is half a gigabyte that never needed to exist.
type Fold struct {
	c    *Combined
	seen []bool
}

// NewFold starts an accumulation over one grid geometry.
func NewFold(south, north, west, east float64, w, h int, freqMHz float64) *Fold {
	n := w * h
	c := &Combined{
		Raster: Raster{South: south, North: north, West: west, East: east,
			Width: w, Height: h, FreqMHz: freqMHz, Cells: make([]Cell, n)},
		BestMarginDB: make([]float64, n),
		BestNode:     make([]int, n),
		ServingCount: make([]int, n),
	}
	for i := 0; i < n; i++ {
		c.BestMarginDB[i] = math.Inf(-1)
		c.BestNode[i] = -1
	}
	return &Fold{c: c, seen: make([]bool, n)}
}

// Add folds one station's raster in, with the same rules Combine applies.
func (f *Fold) Add(r *Raster, station int) {
	for i, cell := range r.Cells {
		if cell.NoData {
			continue
		}
		f.seen[i] = true
		if cell.Workable() {
			f.c.ServingCount[i]++
		}
		m := math.Min(cell.OutboundMarginDB, cell.InboundMarginDB)
		if m > f.c.BestMarginDB[i] {
			f.c.BestMarginDB[i] = m
			f.c.BestNode[i] = station
			f.c.Cells[i] = cell
		}
	}
}

// Done finishes: cells no station had data for are NoData, exactly as
// Combine marks them.
func (f *Fold) Done() *Combined {
	for i := range f.seen {
		if !f.seen[i] {
			f.c.Cells[i] = Cell{NoData: true}
			f.c.BestMarginDB[i] = math.NaN()
		}
	}
	return f.c
}
