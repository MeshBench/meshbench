package engine_test

import (
	"encoding/gob"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/scenario"
)

type fixtureFile struct {
	Name    string          `json:"name"`
	FreqMHz float64         `json:"freq_mhz"`
	Seed    uint64          `json:"seed"`
	Nodes   []scenario.Node `json:"nodes"`
}

// Reports how close a real fixture's links sit to the edge of decoding.
//
// Not an assertion about the fixture - it is a measurement of one, and the
// number it prints decides whether a receiver-sensitivity study can measure
// anything on that scenario at all. A mesh whose links all clear threshold by
// tens of decibels returns the same delivery counts whatever the receiver does.
//
// It reads a link matrix the app has already computed, because recomputing one
// needs terrain tiles and takes minutes; a machine that has never opened the
// fixture has nothing to measure and the test says so rather than inventing
// flat earth, which would answer a different question convincingly.
func TestHowCloseTheLinksAreToThreshold(t *testing.T) {
	for _, name := range []string{"fixture-scotland-strict", "fixture-fife-strict"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "..", "fixtures", name+".json"))
			if err != nil {
				t.Skipf("no fixture: %v", err)
			}
			var f fixtureFile
			if err := json.Unmarshal(raw, &f); err != nil {
				t.Fatalf("fixture: %v", err)
			}

			r := f.Nodes[0].Radio
			e := engine.New(noTerrain{}, engine.Config{
				FreqMHz: f.FreqMHz, SF: r.SpreadFactor, BandwidthHz: float64(r.BandwidthHz),
				CodingRate: r.CodingRate, NoiseFigDB: 6, StepMs: 10, Seed: f.Seed,
			})
			for _, n := range f.Nodes {
				e.Add(n, nil)
			}

			m := savedMatrixFor(t, len(f.Nodes))
			if m == nil {
				t.Skip("no saved link matrix for this node count - open the fixture in the app first")
			}
			e.RestoreLinkCache(m)

			ms := e.LinkMargins()
			if len(ms) == 0 {
				t.Fatal("no directions with a path")
			}
			s := engine.Spread(ms, 2)
			t.Logf("%s: %d nodes, SF%d BW%.0f kHz", f.Name, len(f.Nodes), r.SpreadFactor,
				float64(r.BandwidthHz)/1000)
			t.Logf("  %d directions with a path, %d decoding, median margin %.1f dB, p10 %.1f dB",
				s.Directions, s.Decoding, s.MedianDB, s.P10DB)
			for _, d := range []float64{1, 2, 3, 6} {
				sd := engine.Spread(ms, d)
				t.Logf("  within %.0f dB of threshold: %d of %d decoding (%.2f%%)",
					d, sd.Sensitive, sd.Decoding, 100*sd.Fraction())
			}

			var mg []float64
			for _, x := range ms {
				if x.MarginDB >= 0 {
					mg = append(mg, x.MarginDB)
				}
			}
			sort.Float64s(mg)
			for i := 0; i <= 10; i++ {
				t.Logf("    p%-3d %7.1f dB", i*10, mg[i*(len(mg)-1)/10])
			}
		})
	}
}

// savedMatrixFor finds the app's cached matrix whose indices are exactly this
// many nodes. One fewer is a different scenario; one more is a superset whose
// indices mean other nodes entirely.
func savedMatrixFor(t *testing.T, n int) map[[2]int]float64 {
	t.Helper()
	dir := filepath.Join(os.Getenv("HOME"), ".cache", "meshcoresim", "matrix")
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var best map[[2]int]float64
	for _, en := range ents {
		if filepath.Ext(en.Name()) != ".gob" {
			continue
		}
		fh, err := os.Open(filepath.Join(dir, en.Name()))
		if err != nil {
			continue
		}
		var m map[[2]int]float64
		err = gob.NewDecoder(fh).Decode(&m)
		_ = fh.Close()
		if err != nil {
			continue
		}
		max := -1
		for k := range m {
			if k[1] > max {
				max = k[1]
			}
		}
		if max == n-1 && len(m) > len(best) {
			best = m
		}
	}
	return best
}
