package coverage

import (
	"math"
	"testing"
)

// Fold must be Combine, one raster at a time: same cells, same best server,
// same counts, whatever the order of arrival.
func TestFoldMatchesCombine(t *testing.T) {
	mk := func(seed float64) *Raster {
		r := &Raster{South: 56, North: 56.2, West: -3.4, East: -3.0,
			Width: 12, Height: 9, FreqMHz: 869.618,
			Cells: make([]Cell, 12*9)}
		for i := range r.Cells {
			out := seed - float64(i%7)
			in := seed - float64(i%5) - 1
			r.Cells[i] = Cell{OutboundMarginDB: out, InboundMarginDB: in}
			if i%11 == 0 {
				r.Cells[i] = Cell{NoData: true}
			}
		}
		return r
	}
	rs := []*Raster{mk(6), mk(3), mk(9)}
	ref, err := Combine(rs)
	if err != nil {
		t.Fatal(err)
	}
	f, err := NewFold(56, 56.2, -3.4, -3.0, 12, 9, 869.618)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range rs {
		f.Add(r, i)
	}
	got := f.Done()
	for i := range ref.Cells {
		if ref.Cells[i].NoData != got.Cells[i].NoData ||
			got.BestNode[i] != ref.BestNode[i] ||
			got.ServingCount[i] != ref.ServingCount[i] {
			t.Fatalf("cell %d diverged: fold {node %d n %d} ref {node %d n %d}",
				i, got.BestNode[i], got.ServingCount[i], ref.BestNode[i], ref.ServingCount[i])
		}
		if !ref.Cells[i].NoData &&
			math.Abs(got.BestMarginDB[i]-ref.BestMarginDB[i]) > 1e-12 {
			t.Fatalf("cell %d margin: fold %f ref %f", i, got.BestMarginDB[i], ref.BestMarginDB[i])
		}
	}
}
