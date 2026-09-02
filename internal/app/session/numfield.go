// Reading one number out of a verb's parameter, where absence is an answer.
//
// The numeric half of stringfield.go, and it keeps the same rule for the same
// reason: a bare value is one parameter, so exactly one field may be read from
// it. Kept in its own file beside that one, because the rule is the thing worth
// finding and it used to be buried at the end of the sim-control verbs.
package session

import "encoding/json"

// numField reads a verb's PRIMARY number: the one, and only one, field a bare
// number is allowed to mean.
//
// Use it once per verb, exactly as stringField is used once per verb. A bare
// number satisfies it whatever field it is asked for, so nodes.place reading
// both lat and lon through it turned a bare 5 into a node at 5N 5E - two
// coordinates invented from one. Everything after the primary goes through
// namedNum, and numfield_test.go reads the handlers to keep it that way.
//
// Every kind of integer, not just the float a decoder produces. The control
// socket arrives as JSON and every number in it is a float64, so a map that
// only understood floats worked perfectly for anything scripted - and refused
// the interface, which calls the same verbs in process and passes an int like
// any Go caller would. The symptom was a drawn screen that could not be
// tapped and drawn buttons that could not be pressed, both of them answering
// "needs a node and a point" to a call that had one.
func numField(p any, name string) (float64, bool) {
	if m, ok := p.(map[string]any); ok {
		return toNumber(m[name])
	}
	return toNumber(p)
}

// namedNum reads a number that has to be named to be meant.
//
// No bare-number fallback, for the reason namedField has no bare-string one: a
// caller who sent a bare value has already spent it on the verb's primary
// field, and handing the same number to a second field invents a parameter they
// never passed - and a coordinate, a pin or a channel invented that way is
// accepted, acted on, and reported back as though it had been asked for.
func namedNum(p any, name string) (float64, bool) {
	m, ok := p.(map[string]any)
	if !ok {
		return 0, false
	}
	return toNumber(m[name])
}

// toNumber is the one place that says what counts as a number here.
func toNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}
