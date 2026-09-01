// Parameters a verb refuses rather than quietly replaces.
//
// A default belongs to a parameter that was not supplied. "unchanged when
// absent" is a documented answer and callers depend on it, so a missing
// optional parameter is not an error. A parameter that *was* supplied and
// could not be understood has no such answer, and substituting a default there
// is the worst refusal a verb can make: the caller is told the thing they asked
// for happened, and something else happened instead.
//
// These readers keep the two cases apart - "not supplied" against "supplied and
// unusable" - so a handler can say which it is in. Every refusal here names the
// verb, the parameter, and what would have been accepted, because a message
// that says only "bad request" leaves the caller guessing at exactly the moment
// they have already guessed wrong once.
package session

import (
	"math"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// namesOf reads a set of node names out of a verb's whole parameter.
//
// Four shapes reach the selection verbs: the []string the map's own selection
// passes in process, a single name, the []any a JSON list arrives as over the
// control socket, and the {"names": [...]} object somebody writes because every
// other verb here takes an object. A shape outside that set is refused rather
// than read as an empty list: these verbs apply what they are given to every
// node in the network, so a parameter that parsed as nothing would deselect
// everything and report success.
//
// A parameter that is absent altogether is the one legitimate empty: clearing
// the selection is a thing to want, and it is what "no names" means.
func namesOf(verb string, p any) ([]string, error) {
	switch v := p.(type) {
	case nil:
		return nil, nil
	case []string:
		return v, nil
	case string:
		return []string{v}, nil
	case []any:
		return stringsIn(verb, "", v)
	case map[string]any:
		return namesInObject(verb, v)
	}
	return nil, badParams("%s takes node names: a list, one name, or "+
		`{"names": [...]}; it was given %T`, verb, p)
}

// namesInObject is the object form, kept apart so namesOf stays one switch.
func namesInObject(verb string, m map[string]any) ([]string, error) {
	raw, ok := m["names"]
	if !ok {
		return nil, badParams("%s was given an object with no %q in it; "+
			`it takes a list of names, one name, or {"names": [...]}`, verb, "names")
	}
	switch n := raw.(type) {
	case nil:
		return nil, nil
	case []string:
		return n, nil
	case string:
		return []string{n}, nil
	case []any:
		return stringsIn(verb, "names", n)
	}
	return nil, badParams("%s: names is a %T; it takes a list of node names",
		verb, raw)
}

// stringsIn turns a JSON list into names, refusing a member that is not one.
//
// One bad member fails the call rather than being dropped: a list of forty
// names with a number in the middle is a caller's mistake, and selecting the
// thirty-nine is a silently different answer to the question asked.
func stringsIn(verb, field string, in []any) ([]string, error) {
	out := make([]string, 0, len(in))
	for i, x := range in {
		s, ok := x.(string)
		if !ok {
			where := verb
			if field != "" {
				where = verb + ": " + field
			}
			return nil, badParams("%s member %d is a %T; every one must be a node name",
				where, i, x)
		}
		out = append(out, s)
	}
	return out, nil
}

// unknownNames refuses the names this network has not got, and says what it
// has instead.
//
// Naming what is available is the difference between a refusal somebody can act
// on and one they have to go and investigate: a typo and a stale script look
// identical until the message lists the nodes that do exist.
func unknownNames(verb string, nodes []state.Node, names []string) error {
	have := make(map[string]bool, len(nodes))
	all := make([]string, 0, len(nodes))
	for _, n := range nodes {
		have[n.Name] = true
		all = append(all, n.Name)
	}
	var missing []string
	for _, n := range names {
		if !have[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if len(all) == 0 {
		return badParams("%s: this network has no nodes; place or import some first", verb)
	}
	return badParams("%s: no node named %s; there is: %s",
		verb, nameList(missing), nameList(all))
}

// numAsked reads a number that may legitimately be absent.
//
// Three answers rather than two: not asked for, asked for and usable, asked for
// and not. Collapsing the first and the third is the whole bug this file is
// about - a verb that could not read `"hours": "twelve"` went on to use its own
// 24 and reported the window it had chosen as the window it had been given.
func numAsked(verb, name string, p any) (float64, bool, error) {
	switch v := p.(type) {
	case nil:
		return 0, false, nil
	case map[string]any:
		raw, present := v[name]
		if !present || raw == nil {
			return 0, false, nil
		}
		n, ok := toNumber(raw)
		if !ok {
			return 0, false, badParams("%s: %s is a %T; it takes a number",
				verb, name, raw)
		}
		return n, true, nil
	}
	// A bare value: several of these verbs take their one number that way, and
	// anything that is not a number is a parameter meant for some other field.
	n, ok := toNumber(p)
	return n, ok, nil
}

// requiredNum reads a number a verb cannot proceed without.
//
// Both halves refuse: absent, because there is no sensible stand-in for a
// coordinate; and out of range, because a latitude of 560 is a caller who meant
// something else, and storing it puts a node somewhere no node can be.
func requiredNum(verb, name string, p any, lo, hi float64) (float64, error) {
	v, asked, err := numAsked(verb, name, p)
	if err != nil {
		return 0, err
	}
	if !asked {
		return 0, badParams("%s needs %s, a number from %g to %g", verb, name, lo, hi)
	}
	return inRange(verb, name, v, lo, hi)
}

// numInRange reads a number that has a documented default when absent, and
// refuses it when it is present and unusable.
//
// The distinction this file exists for, in one function: absent returns the
// caller's default with no complaint, present-and-wrong returns an error rather
// than that same default.
func numInRange(verb, name string, p any, def, lo, hi float64) (float64, error) {
	v, asked, err := numAsked(verb, name, p)
	if err != nil {
		return 0, err
	}
	if !asked {
		return def, nil
	}
	return inRange(verb, name, v, lo, hi)
}

func inRange(verb, name string, v, lo, hi float64) (float64, error) {
	if math.IsNaN(v) || v < lo || v > hi {
		return 0, badParams("%s: %s is %v, which is outside %g to %g",
			verb, name, v, lo, hi)
	}
	return v, nil
}
