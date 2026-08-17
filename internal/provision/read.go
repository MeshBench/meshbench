package provision

import "strings"

// NodeState is what is actually true of a node, from two sources: what the
// scenario/import says it should be (Imported), and what the firmware just
// answered (Values, Regions, DefaultScope, UnscopedFlood). Conditions and
// diffing are evaluated against the firmware's answer, never the import - a
// node is the authority on what it is.
type NodeState struct {
	Name     string
	Kind     string
	Lat, Lon float64
	Selected bool
	// Areas is every study area this node's position falls inside, by name.
	Areas []string
	// Companion is true for a node that speaks the app protocol rather than
	// this CLI - loop.detect and CAD have no equivalent there, and a
	// condition on them must not match one.
	Companion bool
	// Read is false until a readback has actually run for this node - a
	// field with no value is not the same as a field read as empty, and
	// treating "not yet asked" as "false" or "empty" is how a rule matches
	// nodes it should not.
	Read bool

	// Regions is what the node currently holds and allows to flood - read
	// from a bare `region` command, not from the import.
	Regions []string
	// DefaultScope is what `region default` reports; empty means unscoped.
	DefaultScope string
	// DefaultScopeKnown distinguishes empty-and-unscoped from not-yet-read.
	DefaultScopeKnown bool
	// UnscopedFlood is whether the wildcard is allowed to flood - what
	// `region allowf *` actually controls, and only that.
	UnscopedFlood bool

	// Values is every readable Key answered so far, keyed by Key.Name, in
	// whatever form the firmware sent it back (after Get's "> " prefix has
	// been stripped).
	Values map[string]string

	// Imported is what the scenario or CoreScope import recorded, kept
	// alongside the readback so "as imported" effects have something to
	// reproduce.
	Imported ImportedFacts
}

// ImportedFacts is the subset of a node's scenario record that provisioning
// can reproduce. Not the whole scenario.Node - only the fields an "as
// imported" effect has a firmware command for.
type ImportedFacts struct {
	Name          string
	Lat, Lon      float64
	Regions       []string
	DefaultScope  string
	AllowAnyFlood bool
}

// RequiredReads is every command a readback needs to send to evaluate a rule
// set and to diff its effects against - the union across every rule, so the
// cost is proportional to what is actually configured rather than to the
// whole 45-key table.
//
// Regions and default scope are always included: almost every study touches
// one or the other, and `region` is one command for the whole table, so
// including it unconditionally costs nothing extra in practice.
func RequiredReads(rules []Rule) []string {
	seen := map[string]bool{"region": true, "region default": true}
	add := func(get string) {
		if get != "" {
			seen[get] = true
		}
	}
	for _, r := range rules {
		for _, c := range r.Conditions {
			if c.Custom {
				add(c.CustomGet)
				continue
			}
			if k, ok := ByName[c.Field]; ok {
				add(k.Get)
			}
		}
		for _, e := range r.Effects {
			if k, ok := ByName[e.Field]; ok {
				add(k.Get)
			}
		}
	}
	out := make([]string, 0, len(seen))
	for cmd := range seen {
		out = append(out, cmd)
	}
	return out
}

// ParseGetReply strips the "> " every handleGetCmd reply carries. A reply
// that does not have it was not a get reply at all - an error, or output from
// something else entirely - and is reported as such rather than accepted.
func ParseGetReply(reply string) (string, bool) {
	reply = strings.TrimSpace(reply)
	v, ok := strings.CutPrefix(reply, "> ")
	if !ok {
		return "", false
	}
	return strings.TrimSpace(v), true
}

// ParseRegionTree reads bare `region`'s output: one line per region, indent
// is depth, an optional trailing "^" marks home, a trailing " F" means flood
// is allowed. Returns the flood-allowed names, "#" stripped - which is what
// every condition and diff in this package means by "regions held".
func ParseRegionTree(lines []string) (regions []string, unscopedFlood bool) {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		allowed := false
		name := trimmed
		if rest, ok := strings.CutSuffix(trimmed, " F"); ok {
			allowed, name = true, rest
		}
		name = strings.TrimSuffix(name, "^")
		if name == "*" {
			unscopedFlood = allowed
			continue
		}
		if !allowed {
			continue
		}
		regions = append(regions, strings.TrimPrefix(name, "#"))
	}
	return regions, unscopedFlood
}

// ParseRegionDefault reads `region default`'s reply: " default scope is X" or
// " default scope is <null>".
func ParseRegionDefault(reply string) (scope string, known bool) {
	reply = strings.TrimSpace(reply)
	v, ok := strings.CutPrefix(reply, "default scope is ")
	if !ok {
		return "", false
	}
	v = strings.TrimSpace(v)
	if v == "<null>" {
		return "", true
	}
	return strings.TrimPrefix(v, "#"), true
}
