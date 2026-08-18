package gpu

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/coverage"
)

// The fold's ADR-0004 harness: three stations - one with a directional
// pattern, so the gain table's interpolation is on trial too - folded on
// the device against the CPU twin folding the CPU kernel's losses. Best
// and second slots must agree cell by cell: same station, same margins.
func TestCoverageFoldMatchesCPU(t *testing.T) {
	d, err := Open()
	if err != nil {
		t.Skipf("no GPU available: %v", err)
	}
	defer d.Close()

	g := hilly()
	base := coverage.GridLossParams{
		RasterW: 96, RasterH: 96,
		South: 56.5, North: 57.0, West: -4.2, East: -3.2,
		RemoteHeightM: 1.5, FreqMHz: 869.525, Steps: 200,
	}
	stations := []struct {
		lat, lon, alt float64
		budget        coverage.StationBudget
		gain          func(b, e float64) float64
	}{
		{56.75, -3.7, 620, coverage.StationBudget{TxPowerDBm: 22, SensitivityDBm: -124,
			RemoteTxDBm: 20, RemoteSensitivityDBm: -124, Station: 0},
			func(b, e float64) float64 { return 2.15 }},
		{56.6, -4.0, 340, coverage.StationBudget{TxPowerDBm: 27, SensitivityDBm: -121,
			RemoteTxDBm: 20, RemoteSensitivityDBm: -124, Station: 1},
			// A beam: the fold must rank stations differently on and off it.
			func(b, e float64) float64 {
				return 9 - 12*math.Pow(math.Sin((b-45)*math.Pi/360), 2) - 0.1*math.Abs(e)
			}},
		{56.9, -3.4, 415, coverage.StationBudget{TxPowerDBm: 14, SensitivityDBm: -129,
			RemoteTxDBm: 20, RemoteSensitivityDBm: -124, Station: 2},
			func(b, e float64) float64 { return 5.0 }},
	}

	cg, err := d.UploadGrid(g)
	if err != nil {
		t.Fatal(err)
	}
	defer cg.Release()
	fold, err := cg.NewFold(base.RasterW, base.RasterH)
	if err != nil {
		t.Fatal(err)
	}
	defer fold.Release()

	cells := base.RasterW * base.RasterH
	wantBest := coverage.NewFoldSlots(cells)
	wantSecond := coverage.NewFoldSlots(cells)
	wantServed := make([]uint32, cells)

	for _, st := range stations {
		p := base
		p.StLat, p.StLon, p.StAltM = st.lat, st.lon, st.alt
		gt := coverage.SampleGains(st.gain)
		if err := fold.Station(p, st.budget, gt); err != nil {
			t.Fatal(err)
		}
		coverage.FoldStationCPU(coverage.GridLossCPU(g, p), g, p, st.budget, gt,
			wantBest, wantSecond, wantServed)
	}
	gotBest, gotSecond, gotServed, err := fold.Read()
	if err != nil {
		t.Fatal(err)
	}

	check := func(name string, got, want []coverage.FoldSlot) {
		wrong := 0
		for i := range want {
			if got[i].Station != want[i].Station {
				// A rank swap at a near-tie is float noise, not a defect:
				// accept it only when the margins are within noise of tied.
				if math.Abs(float64(got[i].MinDB-want[i].MinDB)) > 0.05 {
					wrong++
				}
				continue
			}
			if math.Abs(float64(got[i].MinDB-want[i].MinDB)) > 0.05 ||
				math.Abs(float64(got[i].OutDB-want[i].OutDB)) > 0.05 ||
				math.Abs(float64(got[i].InDB-want[i].InDB)) > 0.05 {
				wrong++
			}
		}
		if wrong > 0 {
			t.Fatalf("%s: %d of %d cells disagree with the CPU twin", name, wrong, len(want))
		}
	}
	check("best", gotBest, wantBest)
	check("second", gotSecond, wantSecond)
	for i := range wantServed {
		if gotServed[i] != wantServed[i] {
			t.Fatalf("served[%d]: gpu %d, cpu %d", i, gotServed[i], wantServed[i])
		}
	}
}
