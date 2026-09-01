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

// soleString reads a verb's one parameter, which arrives either as a bare
// value or as the single-key object the old socket's callers write.
//
// Read here rather than unwrapped at the socket, because the socket cannot
// know which parameter a single key was meant to be.
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
