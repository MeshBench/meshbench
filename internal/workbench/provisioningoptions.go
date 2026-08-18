// The vocabulary a rule can be written in: which fields exist, which
// operators make sense for each, and what an effect on a field may do.
package workbench

import (
	"strings"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

// conditionFieldOptions is every field a condition can be written against:
// the pseudo-fields the interface itself knows, plus every readable key in
// the firmware's command table - never a hand-picked subset, so a condition
// can never be written against a key that turns out not to exist.
func conditionFieldOptions(s *state.Snapshot) []string {
	out := append([]string(nil), pseudoConditionFields...)
	if s != nil {
		for _, k := range s.ProvisioningKeys {
			out = append(out, k.Name)
		}
	}
	return out
}

func effectFieldOptions(s *state.Snapshot) []string {
	out := append([]string(nil), pseudoEffectFields...)
	if s != nil {
		for _, k := range s.ProvisioningKeys {
			out = append(out, k.Name)
		}
	}
	out = append(out, "(custom command)")
	return out
}

// conditionOpOptions is the legal comparisons for a field, which differ by
// shape: a list of regions is asked whether it contains something, a scalar
// is asked whether it equals something.
func conditionOpOptions(field string) []string {
	switch field {
	case "regions":
		return []string{"contain", "do not contain", "are only", "are empty"}
	case "unscoped-flood", "selected":
		return []string{"is"}
	case "default-scope", "kind", "area", "name":
		return []string{"is", "is not", "contains", "is empty"}
	}
	if strings.HasPrefix(field, "(custom)") {
		return []string{"is", "is not", "contains"}
	}
	return []string{"is", "is not", "contains", "less than", "greater than"}
}

// effectModeOptions is what an effect on a field may do: regions can only
// ever add to what a node holds in this panel - see the design note on why
// destructive replace is not offered here - and a scalar is either
// reproduced from the import or set outright.
func effectModeOptions(field string) []string {
	switch field {
	case "regions":
		return []string{"leave", "asimported", "add"}
	case "(custom command)":
		return []string{"set"}
	case "name", "lat", "lon", "default-scope":
		return []string{"leave", "asimported", "set"}
	}
	return []string{"leave", "set"}
}
