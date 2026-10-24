// What experiment.define keeps of an arm, and what it refuses.
package experiment

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/app/session"
)

// An arm defined with cad on used to run with cad off: define read the label
// and the two firmware versions and dropped everything else, so the arm was a
// second control arm under a label saying otherwise.
func TestADefinedArmKeepsEverySettingItNames(t *testing.T) {
	st, s := aNetwork(t)
	_, err := st.Do(t.Context(), "experiment.define", map[string]any{
		"arms": []any{
			map[string]any{"label": "base", "repeater_version": "v1.18.0"},
			map[string]any{
				"label": "listens", "repeater_version": "v1.18.0", "cad": "on",
				"loop_detect": "strict", "path_hash_mode": float64(0),
				"spread_ms": float64(5000), "set": map[string]any{"agc.reset.interval": "4"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	arms := matrixOf(s).Arms
	if len(arms) != 2 {
		t.Fatalf("defined %d arms, want 2", len(arms))
	}
	a := arms[1]
	if a.CAD != "on" || a.LoopDetect != "strict" || a.Set["agc.reset.interval"] != "4" {
		t.Fatalf("the arm lost its settings: %+v", a)
	}
	if a.PathHashMode == nil || *a.PathHashMode != 0 {
		t.Fatalf("path hash mode 0 did not survive: %v", a.PathHashMode)
	}
	if a.SpreadMs == nil || *a.SpreadMs != 5000 {
		t.Fatalf("spread did not survive: %v", a.SpreadMs)
	}

	var p session.Provisioning
	a.ApplyOver(&p)
	if p.CadMode != "on" {
		t.Errorf("provisioning for the arm has cad %q", p.CadMode)
	}
	if arms[0].CAD != "" {
		t.Errorf("the control arm picked up cad %q", arms[0].CAD)
	}
}

// A misspelt field, or a value the setting cannot take, is the same silent
// control arm, so both are refused and the arms already defined are kept.
func TestADefinedArmWithAFieldItCannotUseIsRefused(t *testing.T) {
	st, s := aNetwork(t)
	if _, err := st.Do(t.Context(), "experiment.define", map[string]any{
		"arms": []any{map[string]any{"label": "kept"}},
	}); err != nil {
		t.Fatal(err)
	}
	for _, arm := range []map[string]any{
		{"label": "x", "cad_mode": "on"},
		{"label": "x", "cad": "maybe"},
		{"label": "x", "path_hash_mode": float64(7)},
		{"label": "x", "spread_ms": float64(-1)},
	} {
		_, err := st.Do(t.Context(), "experiment.define", map[string]any{"arms": []any{arm}})
		if err == nil {
			t.Errorf("define accepted %v", arm)
			continue
		}
		if code := control.CodeOf(err); code != control.BadParams {
			t.Errorf("define refused %v as %q, want %q", arm, code, control.BadParams)
		}
	}
	mentions(t, refuses(t, st, "experiment.define", map[string]any{
		"arms": []any{map[string]any{"cad_mode": "on"}},
	}), "cad_mode")
	if arms := matrixOf(s).Arms; len(arms) != 1 || arms[0].Label != "kept" {
		t.Errorf("a refused define replaced the arms with %+v", arms)
	}
}
