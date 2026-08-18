// The firmware's own command table, handed to the panel so it can build a
// chooser instead of a hand-typed box - see internal/provision/keys.go for
// where the table itself comes from.
package session

import (
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/provision"
)

func kindLabel(k provision.Kind) string {
	switch k {
	case provision.KindInt:
		return "int"
	case provision.KindBool:
		return "bool"
	case provision.KindEnum:
		return "enum"
	}
	return "string"
}

func toStateKey(k provision.Key) state.ProvisionKey {
	out := state.ProvisionKey{
		Name: k.Name, Kind: kindLabel(k.Kind), Enum: k.Enum,
		Companion: k.CompanionField != "", Note: k.Note,
	}
	if k.Min != nil {
		out.Min, out.HasMin = *k.Min, true
	}
	if k.Max != nil {
		out.Max, out.HasMax = *k.Max, true
	}
	return out
}

func provisioningKeys() []state.ProvisionKey {
	out := make([]state.ProvisionKey, len(provision.Table))
	for i, k := range provision.Table {
		out[i] = toStateKey(k)
	}
	return out
}

func registerProvisioningKeys(st *state.Store, s *Sim) {
	st.Handle("provisioning.keys", func(w *state.World, _ any) (any, error) {
		keys := provisioningKeys()
		w.ProvisioningKeys = keys
		return map[string]any{"keys": len(keys)}, nil
	})
}
