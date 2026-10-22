package gpu

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/propagation"
)

func TestLossComparisonRejectsInvalidReadback(t *testing.T) {
	for _, tc := range []struct {
		name      string
		got, want []float32
	}{
		{"short", nil, []float32{1}},
		{"long", []float32{1, 2}, []float32{1}},
		{"NaN", []float32{float32(math.NaN())}, []float32{1}},
		{"infinity", []float32{float32(math.Inf(1))}, []float32{1}},
		{"missing terrain", []float32{propagation.NoDataLoss}, []float32{1}},
		{"invented terrain", []float32{1}, []float32{propagation.NoDataLoss}},
		{"wrong loss", []float32{2}, []float32{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := lossDifference(tc.got, tc.want, 0.1); err == nil {
				t.Fatal("an invalid result must not pass the equivalence test")
			}
			if err := compare("test", tc.got, tc.want); err == nil {
				t.Fatal("an invalid result must not pass the runtime self-check")
			}
		})
	}
	if _, err := lossDifference([]float32{1.01, propagation.NoDataLoss},
		[]float32{1, propagation.NoDataLoss}, 0.1); err != nil {
		t.Fatal(err)
	}
}
