package provision

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ResolvedCommand is one line of a node's script, with which rule asked for
// it - the annotation the resolved preview needs, since with several rules in
// play a node's script is no longer something an operator can work out by
// reading the panel.
type ResolvedCommand struct {
	Command  string
	RuleName string
}

// Resolve turns a rule list and a node's readback into exactly the commands
// that change something. A rule with no matching conditions contributes
// nothing; a rule whose effect already matches what the node holds
// contributes nothing either - reconciliation, not scripting, so a second run
// against an unchanged node sends nothing at all.
func Resolve(rules []Rule, ns NodeState) []ResolvedCommand {
	var out []ResolvedCommand
	matched := make([]Rule, 0, len(rules))
	for _, r := range rules {
		if r.Matches(ns) {
			matched = append(matched, r)
		}
	}

	out = append(out, resolveRegions(matched, ns)...)
	out = append(out, resolveDefaultScope(matched, ns)...)
	out = append(out, resolveUnscopedFlood(matched, ns)...)
	out = append(out, resolveScalars(matched, ns)...)
	out = append(out, resolveCustom(matched)...)
	return out
}

// resolveRegions unions every matching rule's regions effect onto what the
// node was imported holding, then emits only the ones the node does not
// already carry - "as imported" and "add" differ only in where the tokens
// come from, and both end up in the same union.
func resolveRegions(rules []Rule, ns NodeState) []ResolvedCommand {
	desired := map[string]bool{}
	touched := false
	contributor := map[string]string{}
	for _, r := range rules {
		for _, e := range r.Effects {
			if e.Field != "regions" {
				continue
			}
			switch e.Mode {
			case ModeAsImported:
				touched = true
				for _, tok := range ns.Imported.Regions {
					desired[tok] = true
					if contributor[tok] == "" {
						contributor[tok] = r.Name
					}
				}
			case ModeAdd:
				touched = true
				for _, tok := range splitTokens(e.Value) {
					desired[tok] = true
					if contributor[tok] == "" {
						contributor[tok] = r.Name
					}
				}
			}
		}
	}
	if !touched {
		return nil
	}
	var add []string
	for tok := range desired {
		if !containsFold(ns.Regions, tok) {
			add = append(add, tok)
		}
	}
	if len(add) == 0 {
		return nil
	}
	sort.Strings(add)
	var out []ResolvedCommand
	for _, tok := range add {
		who := contributor[tok]
		out = append(out,
			ResolvedCommand{Command: "region put " + tok, RuleName: who},
			ResolvedCommand{Command: "region allowf " + tok, RuleName: who})
	}
	out = append(out, ResolvedCommand{Command: "region save"})
	return out
}

func resolveDefaultScope(rules []Rule, ns NodeState) []ResolvedCommand {
	mode, value, who := "", "", ""
	for _, r := range rules {
		for _, e := range r.Effects {
			if e.Field != "default-scope" || e.Mode == ModeLeave {
				continue
			}
			mode, value, who = e.Mode, e.Value, r.Name
		}
	}
	if mode == "" {
		return nil
	}
	if mode == ModeAsImported {
		value = ns.Imported.DefaultScope
	}
	if ns.DefaultScopeKnown && strings.EqualFold(ns.DefaultScope, value) {
		return nil
	}
	target := value
	if target == "" {
		target = "<null>"
	}
	return []ResolvedCommand{{Command: "region default " + target, RuleName: who}}
}

func resolveUnscopedFlood(rules []Rule, ns NodeState) []ResolvedCommand {
	set, want, who := false, false, ""
	for _, r := range rules {
		for _, e := range r.Effects {
			if e.Field != "unscoped-flood" || e.Mode == ModeLeave {
				continue
			}
			set = true
			want = e.Value == "relay" || e.Value == "yes" || e.Value == "allow"
			who = r.Name
		}
	}
	if !set || want == ns.UnscopedFlood {
		return nil
	}
	cmd := "region denyf *"
	if want {
		cmd = "region allowf *"
	}
	return []ResolvedCommand{{Command: cmd, RuleName: who}}
}

// resolveScalars is every Key.Name effect: name, lat, lon, path.hash.mode,
// loop.detect, and the rest of the table. Last matching rule wins, because
// each of these is one `set` command with one value - there is no union to
// take, unlike regions.
func resolveScalars(rules []Rule, ns NodeState) []ResolvedCommand {
	type chosen struct{ mode, value, who string }
	desired := map[string]chosen{}
	for _, r := range rules {
		for _, e := range r.Effects {
			if e.Mode == ModeLeave {
				continue
			}
			if _, ok := ByName[e.Field]; !ok {
				continue
			}
			desired[e.Field] = chosen{e.Mode, e.Value, r.Name}
		}
	}
	names := make([]string, 0, len(desired))
	for name := range desired {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []ResolvedCommand
	for _, name := range names {
		c := desired[name]
		key := ByName[name]
		if key.CompanionField == "" && ns.Companion {
			continue
		}
		value := c.value
		if c.mode == ModeAsImported {
			var ok bool
			value, ok = importedValue(name, ns.Imported)
			if !ok {
				continue
			}
		}
		wire := wireValue(name, value)

		// The clock cannot be diffed against what `clock` replies with - see
		// Key{Name: "clock"}'s own note - so it is sent whenever asked for,
		// and a refusal shows up in the transcript rather than being
		// silently skipped here.
		if name != "clock" {
			if cur, known := ns.Values[name]; known && strings.EqualFold(cur, wire) {
				continue
			}
		}
		out = append(out, ResolvedCommand{Command: key.Set + " " + wire, RuleName: c.who})
	}
	return out
}

func resolveCustom(rules []Rule) []ResolvedCommand {
	var out []ResolvedCommand
	for _, r := range rules {
		for _, e := range r.Effects {
			if e.Field == "" && e.Mode == ModeSet && e.CustomSet != "" {
				out = append(out, ResolvedCommand{Command: e.CustomSet, RuleName: r.Name})
			}
		}
	}
	return out
}

// importedValue is the scenario's own value for a field, for the few fields
// an import actually carries. Anything else has no import to reproduce, so
// "as imported" on it is a no-op rather than a guess.
func importedValue(field string, imp ImportedFacts) (string, bool) {
	switch field {
	case "name":
		return imp.Name, imp.Name != ""
	case "lat":
		return fmt.Sprintf("%.6f", imp.Lat), true
	case "lon":
		return fmt.Sprintf("%.6f", imp.Lon), true
	}
	return "", false
}

// wireValue translates a display value to what the firmware actually wants
// on the wire, for the one field where they differ: path.hash.mode is offered
// in bytes and sent as bytes minus one.
func wireValue(field, display string) string {
	if field == "path.hash.mode" {
		if n, err := strconv.Atoi(strings.TrimSpace(display)); err == nil {
			return strconv.Itoa(n - 1)
		}
	}
	return display
}

func containsFold(list []string, tok string) bool {
	for _, l := range list {
		if strings.EqualFold(l, tok) {
			return true
		}
	}
	return false
}
