// Reading one field out of a verb's parameter, where absence is an answer.
//
// The lenient half of the parameter readers: params.go refuses a value that
// was supplied and could not be understood, and these three say only whether a
// field was there, because "unchanged when absent" is what most of the verbs
// promise. The rule they exist to keep is which single field a bare value is
// allowed to mean, and stringfield_test.go reads the handlers to enforce it.
package session

// stringField reads a verb's PRIMARY field: the one, and only one, that a bare
// string parameter is allowed to mean.
//
// Use it once per verb. Every other field goes through namedField, because a
// bare string satisfies this function whatever it is asked for - ask it for two
// fields and both come back holding the same value. That is how node.window
// came to be unopenable: the window is asked for by name, the name arrived as a
// bare string, and the optional tab was read with this, so every double-click
// asked for a tab named after the node and was refused.
func stringField(p any, name string) (string, bool) {
	switch v := p.(type) {
	case map[string]any:
		s, ok := v[name].(string)
		return s, ok
	case string:
		return v, true
	}
	return "", false
}

// namedField reads a field that has to be named to be meant.
//
// No bare-string fallback: a caller who wrote one has already spent it on the
// verb's primary field, and handing the same value to a second field invents a
// parameter they did not pass.
func namedField(p any, name string) (string, bool) {
	m, ok := p.(map[string]any)
	if !ok {
		return "", false
	}
	s, ok := m[name].(string)
	return s, ok
}

// primaryString reads a verb's primary string parameter by the name it
// documents, and falls back to a lone unnamed value only when that name is not
// there.
//
// The reader a verb with a documented primary uses, because soleString on its
// own does not look the documented name up at all: it answers with the only
// value of any single-key object, so `{"node": "West Lomond", "region": "tay"}`
// read as no node and was refused by a message naming the empty string, while
// `{"anything": "West Lomond"}` was accepted. A caller who passed exactly what
// the description asked for, plus one thing more, got the refusal.
//
// Both halves are needed. The name is what the description promises; the
// fallback is the single-key object the old socket's callers write, which the
// socket itself cannot unwrap because it does not know which parameter that one
// key was meant to be.
func primaryString(p any, name string) string {
	if s, ok := stringField(p, name); ok && s != "" {
		return s
	}
	return soleString(p)
}

// soleString reads a verb's one parameter, which arrives either as a bare
// value or as the single-key object the old socket's callers write.
//
// For a verb that documents no parameter of its own. Anything that names one
// reads it with primaryString, and paramrules_test.go holds the handlers to it.
func soleString(p any) string {
	switch v := p.(type) {
	case string:
		return v
	case map[string]any:
		if len(v) == 1 {
			for _, only := range v {
				if s, ok := only.(string); ok {
					return s
				}
			}
		}
	}
	return ""
}
