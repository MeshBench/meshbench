package provision

import (
	"strconv"
	"strings"
)

// Condition is one clause of a rule's "when". Conditions and-together within
// a rule; an empty condition list matches every node, which is deliberately
// how a study-wide default is expressed - there is no separate concept of a
// base rule, only a rule with nothing in When.
type Condition struct {
	// Field is a pseudo-field ("regions", "default-scope", "unscoped-flood",
	// "kind", "selected", "area") or a provision.Key.Name.
	Field string
	Op    string
	Value string
	// Custom, when true, ignores Field and matches whatever CustomGet
	// answered instead - the escape hatch for a command this table does not
	// know yet.
	Custom    bool
	CustomGet string
}

// Effect is one clause of a rule's "then".
type Effect struct {
	// Field is "regions", "default-scope", "unscoped-flood", or a
	// provision.Key.Name.
	Field string
	// Mode is "leave" (the default; emits nothing), "asimported" (reproduce
	// what the scenario/import recorded), "add" (regions only - union onto
	// whatever the node already holds), or "set" (Value).
	Mode  string
	Value string
	// CustomSet, when Field is empty and Mode is "set", is a raw command
	// line sent verbatim - the effect side of the same escape hatch.
	CustomSet string
}

const (
	ModeLeave      = "leave"
	ModeAsImported = "asimported"
	ModeAdd        = "add"
	ModeSet        = "set"
)

// Rule is one when/then pair, matched against the readback rather than the
// import - see NodeState's own doc for why.
type Rule struct {
	Name       string
	Conditions []Condition
	Effects    []Effect
}

// Matches reports whether every condition holds for this node. An unread
// node (ns.Read == false) matches only a rule with no conditions at all -
// a condition needs an answer to be evaluated against, and guessing one is
// how a rule ends up touching a node it was never meant to.
func (r Rule) Matches(ns NodeState) bool {
	if len(r.Conditions) == 0 {
		return true
	}
	if !ns.Read {
		return false
	}
	for _, c := range r.Conditions {
		if !c.holds(ns) {
			return false
		}
	}
	return true
}

func (c Condition) holds(ns NodeState) bool {
	if c.Custom {
		v, ok := ns.Values[customKey(c.CustomGet)]
		if !ok {
			return false
		}
		return compareString(v, c.Op, c.Value)
	}
	switch c.Field {
	case "regions":
		return matchList(ns.Regions, c.Op, c.Value)
	case "default-scope":
		if !ns.DefaultScopeKnown {
			return false
		}
		return compareString(ns.DefaultScope, c.Op, c.Value)
	case "unscoped-flood":
		want := c.Value == "yes" || c.Value == "relay" || c.Value == "true"
		return ns.UnscopedFlood == want
	case "kind":
		return compareString(ns.Kind, c.Op, c.Value)
	case "selected":
		want := c.Value == "yes" || c.Value == "true"
		return ns.Selected == want
	case "area":
		for _, a := range ns.Areas {
			if strings.EqualFold(a, c.Value) {
				return c.Op != "outside"
			}
		}
		return c.Op == "outside"
	}
	// A device-only field: no answer means no match, deliberately - see
	// Matches' own note on ns.Read.
	if k, ok := ByName[c.Field]; ok {
		if k.CompanionField == "" && ns.Companion {
			// This node cannot answer, so a condition on it cannot be honestly
			// evaluated as true or false. Treated as not matching rather than
			// as an error: a rule with several conditions should still be
			// judgeable for every node it does apply to.
			return false
		}
		v, ok := ns.Values[c.Field]
		if !ok {
			return false
		}
		return compareString(v, c.Op, c.Value)
	}
	return false
}

// compareString is every scalar operator this package offers. "is not" is
// spelled with the space it is written with everywhere else in this codebase.
func compareString(got, op, want string) bool {
	switch op {
	case "is", "":
		return strings.EqualFold(got, want)
	case "is not":
		return !strings.EqualFold(got, want)
	case "contains":
		return strings.Contains(strings.ToLower(got), strings.ToLower(want))
	case "is empty":
		return got == ""
	case "is not empty":
		return got != ""
	case "starts with":
		return strings.HasPrefix(strings.ToLower(got), strings.ToLower(want))
	case "less than":
		return numLess(got, want)
	case "greater than":
		return numLess(want, got)
	}
	return false
}

func numLess(a, b string) bool {
	af, aerr := strconv.ParseFloat(strings.TrimSpace(a), 64)
	bf, berr := strconv.ParseFloat(strings.TrimSpace(b), 64)
	return aerr == nil && berr == nil && af < bf
}

func matchList(list []string, op, want string) bool {
	has := func(tok string) bool {
		for _, l := range list {
			if strings.EqualFold(l, tok) {
				return true
			}
		}
		return false
	}
	switch op {
	case "contain", "contains", "":
		return has(want)
	case "do not contain", "does not contain":
		return !has(want)
	case "are empty":
		return len(list) == 0
	case "are only":
		wanted := splitTokens(want)
		if len(wanted) != len(list) {
			return false
		}
		for _, w := range wanted {
			if !has(w) {
				return false
			}
		}
		return true
	}
	return false
}

func splitTokens(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
	out := fields[:0]
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// customKey is where a custom condition's answer is stashed in NodeState.Values,
// namespaced so it cannot collide with a real Key.Name.
func customKey(get string) string { return "custom:" + get }
