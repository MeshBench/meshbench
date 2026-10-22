package gpu

import (
	"fmt"
	"math"

	"github.com/MeshBench/meshbench/internal/rf/propagation"
)

// A NaN makes `difference > tolerance` false. Check finiteness before the
// tolerance so a broken readback cannot make the twins appear to agree.
func lossDifference(got, want []float32, tol float64) (float64, error) {
	if len(got) != len(want) {
		return 0, fmt.Errorf("GPU returned %d values, CPU %d", len(got), len(want))
	}
	worst := 0.0
	for i, cpu := range want {
		gpu := got[i]
		if math.IsNaN(float64(gpu)) || math.IsInf(float64(gpu), 0) ||
			math.IsNaN(float64(cpu)) || math.IsInf(float64(cpu), 0) {
			return worst, fmt.Errorf("value %d is non-finite: GPU %g, CPU %g", i, gpu, cpu)
		}
		gpuNoData, cpuNoData := gpu >= propagation.NoDataLoss/2, cpu >= propagation.NoDataLoss/2
		if gpuNoData != cpuNoData {
			return worst, fmt.Errorf("value %d disagrees about missing terrain: GPU %g, CPU %g", i, gpu, cpu)
		}
		if cpuNoData {
			continue
		}
		diff := math.Abs(float64(gpu) - float64(cpu))
		worst = math.Max(worst, diff)
		if diff > tol {
			return worst, fmt.Errorf("value %d differs by %.4f dB (tolerance %.3f)", i, diff, tol)
		}
	}
	return worst, nil
}
