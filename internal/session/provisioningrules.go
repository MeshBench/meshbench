// The rule list: a base with no conditions, plus whatever a study adds.
//
// state (the render-facing package) knows nothing about provision (the rule
// engine), on purpose - state is a leaf, and a rule that means nothing
// without the firmware's own command table is not something the render loop
// should have an opinion about. This file is the translation at the boundary.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/provision"
)

func toStateRule(r provision.Rule) state.ProvisionRule {
	out := state.ProvisionRule{Name: r.Name}
	for _, c := range r.Conditions {
		out.Conditions = append(out.Conditions, state.ProvisionCondition{
			Field: c.Field, Op: c.Op, Value: c.Value,
			Custom: c.Custom, CustomGet: c.CustomGet,
		})
	}
	for _, e := range r.Effects {
		out.Effects = append(out.Effects, state.ProvisionEffect{
			Field: e.Field, Mode: e.Mode, Value: e.Value, CustomSet: e.CustomSet,
		})
	}
	return out
}

func fromStateRule(r state.ProvisionRule) provision.Rule {
	out := provision.Rule{Name: r.Name}
	for _, c := range r.Conditions {
		out.Conditions = append(out.Conditions, provision.Condition{
			Field: c.Field, Op: c.Op, Value: c.Value,
			Custom: c.Custom, CustomGet: c.CustomGet,
		})
	}
	for _, e := range r.Effects {
		out.Effects = append(out.Effects, provision.Effect{
			Field: e.Field, Mode: e.Mode, Value: e.Value, CustomSet: e.CustomSet,
		})
	}
	return out
}

// legacyAsRule turns the old flat Provisioning settings into a rule with no
// conditions - the base every node gets, expressed the same way a study's own
// override is. There is no separate concept of a base rule; this is what
// makes that true rather than merely stated.
func legacyAsRule(p Provisioning) provision.Rule {
	r := provision.Rule{Name: "the session's own settings"}
	add := func(field, mode, value string) {
		r.Effects = append(r.Effects, provision.Effect{Field: field, Mode: mode, Value: value})
	}
	if p.SetName {
		add("name", provision.ModeAsImported, "")
	}
	if p.SetPosition {
		add("lat", provision.ModeAsImported, "")
		add("lon", provision.ModeAsImported, "")
	}
	if p.SetClock {
		add("clock", provision.ModeSet, fmt.Sprintf("%d", scenarioEpoch))
	}
	if mode := p.PathHashMode; mode >= 0 {
		// The legacy field stores the wire value (0..2); rules speak bytes
		// (1..3), so the same +1 the panel used to show is applied here once.
		add("path.hash.mode", provision.ModeSet, fmt.Sprintf("%d", mode+1))
	}
	if p.LoopDetect != "" {
		add("loop.detect", provision.ModeSet, p.LoopDetect)
	}
	if p.CadMode != "" {
		add("cad", provision.ModeSet, p.CadMode)
	}
	if p.AdvertMinutes > 0 {
		mins := p.AdvertMinutes
		if mins < 60 {
			mins = 60
		}
		if mins > 240 {
			mins = 240
		}
		add("advert.interval", provision.ModeSet, fmt.Sprintf("%d", mins))
	}
	if p.FloodMaxAdvert > 0 {
		add("flood.max.advert", provision.ModeSet, fmt.Sprintf("%d", p.FloodMaxAdvert))
	}
	if p.Extra != "" {
		r.Effects = append(r.Effects, provision.Effect{Mode: provision.ModeSet, CustomSet: p.Extra})
	}
	return r
}

// activeRules is the whole rule list a run actually uses: the legacy base
// first, so its effects can still be overridden by a later rule the same way
// two study rules would override each other, then whatever the study added.
func (s *Sim) activeRules() []provision.Rule {
	out := []provision.Rule{legacyAsRule(*s.provisioning())}
	out = append(out, s.rules...)
	return out
}

func registerProvisioningRules(st *state.Store, s *Sim) {
	st.Handle("provisioning.rules.get", func(w *state.World, _ any) (any, error) {
		out := make([]state.ProvisionRule, len(s.rules))
		for i, r := range s.rules {
			out[i] = toStateRule(r)
		}
		return map[string]any{"rules": out}, nil
	})

	// provisioning.rules.set replaces the whole study-added list, the same
	// wholesale-replace shape nodes.regions already uses: an editor holds its
	// own working copy and sends the lot on every change, so there is never a
	// question of which single rule a partial update meant.
	st.Handle("provisioning.rules.set", func(w *state.World, p any) (any, error) {
		m, ok := p.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("provisioning.rules.set needs a rules list")
		}
		raw, ok := m["rules"].([]any)
		if !ok {
			return nil, fmt.Errorf("provisioning.rules.set needs a rules list")
		}
		rules := make([]provision.Rule, 0, len(raw))
		for _, item := range raw {
			sr, err := decodeStateRule(item)
			if err != nil {
				return nil, err
			}
			rules = append(rules, fromStateRule(sr))
		}
		s.rules = rules
		w.ProvisioningRules = toWorldRules(s.activeRules())
		// The readback cache is not invalidated: the rules changed, not what
		// the nodes hold. Re-resolving against the same cache is what lets
		// match counts and the preview update immediately.
		s.refreshMatches(w)
		w.Say(fmt.Sprintf("%d rules; the base plus %d overrides", len(s.activeRules()), len(rules)))
		return map[string]any{"rules": len(rules)}, nil
	})
}

func toWorldRules(rules []provision.Rule) []state.ProvisionRule {
	out := make([]state.ProvisionRule, len(rules))
	for i, r := range rules {
		out[i] = toStateRule(r)
	}
	return out
}

// decodeStateRule reads one rule back out of the loosely-typed params a verb
// receives - the same map[string]any/[]any shape every other list-replacing
// verb in this package decodes by hand.
func decodeStateRule(item any) (state.ProvisionRule, error) {
	m, ok := item.(map[string]any)
	if !ok {
		return state.ProvisionRule{}, fmt.Errorf("a rule must be an object")
	}
	r := state.ProvisionRule{}
	r.Name, _ = m["name"].(string)
	if cs, ok := m["conditions"].([]any); ok {
		for _, ci := range cs {
			cm, ok := ci.(map[string]any)
			if !ok {
				continue
			}
			c := state.ProvisionCondition{}
			c.Field, _ = cm["field"].(string)
			c.Op, _ = cm["op"].(string)
			c.Value, _ = cm["value"].(string)
			c.Custom, _ = cm["custom"].(bool)
			c.CustomGet, _ = cm["custom_get"].(string)
			r.Conditions = append(r.Conditions, c)
		}
	}
	if es, ok := m["effects"].([]any); ok {
		for _, ei := range es {
			em, ok := ei.(map[string]any)
			if !ok {
				continue
			}
			e := state.ProvisionEffect{}
			e.Field, _ = em["field"].(string)
			e.Mode, _ = em["mode"].(string)
			e.Value, _ = em["value"].(string)
			e.CustomSet, _ = em["custom_set"].(string)
			r.Effects = append(r.Effects, e)
		}
	}
	return r, nil
}
