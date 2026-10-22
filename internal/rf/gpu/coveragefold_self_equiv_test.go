package gpu

import (
	"fmt"
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/propagation"
)

// The general raster never lands exactly on a station. That misses fold's
// zero-horizontal-distance branch, where vertical separation and the gain
// table's elevation clamp must still agree with the CPU twin.
func compareFoldSelfCells(t *testing.T, d *Device) {
	g := propagation.HeightGrid{W: 2, H: 2, North: 1, East: 1, Heights: make([]float32, 4)}
	cg, err := d.UploadGrid(g)
	if err != nil {
		t.Fatal(err)
	}
	defer cg.Release()
	for _, alt := range []float64{0, 1.5, 40} {
		t.Run(fmt.Sprintf("altitude-%g", alt), func(t *testing.T) {
			p := propagation.GridLossParams{
				StLat: 0.5, StLon: 0.5, StAltM: alt,
				RasterW: 1, RasterH: 1, North: 1, East: 1,
				RemoteHeightM: 1.5, FreqMHz: 869.525, Steps: 20,
			}
			b := propagation.StationBudget{TxPowerDBm: 20, SensitivityDBm: -120,
				RemoteTxDBm: 18, RemoteGainDBi: 1, RemoteSensitivityDBm: -121, Station: 7}
			gt := propagation.SampleGains(func(bearing, elev float64) float64 {
				return 2 + 0.1*elev + 0.01*bearing
			})
			fold, err := cg.NewFold(1, 1)
			if err != nil {
				t.Fatal(err)
			}
			defer fold.Release()
			if err := fold.Station(p, b, gt); err != nil {
				t.Fatal(err)
			}
			best, second, served, err := fold.Read()
			if err != nil {
				t.Fatal(err)
			}
			wantBest, wantSecond := propagation.NewFoldSlots(1), propagation.NewFoldSlots(1)
			wantServed := make([]uint32, 1)
			propagation.FoldStationCPU(propagation.GridLossCPU(g, p), g, p, b, gt,
				wantBest, wantSecond, wantServed)
			for name, pair := range map[string][2][]propagation.FoldSlot{
				"best": {best, wantBest}, "second": {second, wantSecond},
			} {
				if len(pair[0]) != 1 || pair[0][0].Station != pair[1][0].Station {
					t.Fatalf("%s: GPU %v, CPU %v", name, pair[0], pair[1])
				}
				for _, margins := range [][2]float32{
					{pair[0][0].MinDB, pair[1][0].MinDB},
					{pair[0][0].OutDB, pair[1][0].OutDB},
					{pair[0][0].InDB, pair[1][0].InDB},
				} {
					diff := math.Abs(float64(margins[0]) - float64(margins[1]))
					if math.IsNaN(diff) || math.IsInf(diff, 0) || diff > 0.05 {
						t.Fatalf("%s margin: GPU %g, CPU %g", name, margins[0], margins[1])
					}
				}
			}
			if len(served) != 1 || served[0] != wantServed[0] {
				t.Fatalf("serving counts: GPU %v, CPU %v", served, wantServed)
			}
		})
	}
}
