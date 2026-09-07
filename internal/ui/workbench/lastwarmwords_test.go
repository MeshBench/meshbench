package workbench

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// A warm that was skipped because the matrix already answered every pair is
// not a warm that measured nothing, and not a warm that measured pairs either.
func TestASkippedWarmIsNeitherNotYetNorMeasured(t *testing.T) {
	v, c := lastWarmWords(state.GPUState{Used: true, Pairs: 71253, Ms: 10603})
	if !strings.Contains(v, "71253 pairs") {
		t.Errorf("a real warm reads %q", v)
	}
	if !strings.Contains(c, "actually did") {
		t.Errorf("a real warm's caption is %q", c)
	}

	v, c = lastWarmWords(state.GPUState{Why: "the matrix already answered every pair"})
	if strings.Contains(v, "pairs") || v == "not yet" {
		t.Errorf("a skipped warm reads %q, which is a measurement or an absence", v)
	}
	if !strings.Contains(c, "already answered") {
		t.Errorf("a skipped warm does not carry its reason: %q", c)
	}

	v, _ = lastWarmWords(state.GPUState{})
	if v != "not yet" {
		t.Errorf("an untouched device reads %q, want not yet", v)
	}
}
