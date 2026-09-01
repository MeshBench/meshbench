package state

import "sort"

// Who may call a verb, said where it is registered.
//
// Most verbs are a request: place a node, run for ten seconds, compute a
// raster. A few are the opposite - a goroutine inside this process handing a
// finished result back to the store, on the one thread allowed to apply it.
// Those take Go values nothing outside the process can spell: a *Coverage, a
// []Link, a slice of some package's private struct. Reached from the control
// socket they arrive as JSON, the type assertion misses, and the handler
// applies the zero value: a fetched catalogue replaced with nothing, and a
// success returned for it.
//
// So the decision is written down at the registration rather than kept as a
// list somewhere else. A list would be a second thing to maintain, and the
// first thing it would do is fall behind.

// HandleInternal registers a verb the application calls itself and nobody
// outside it may. It is otherwise exactly Handle: the interface and the store's
// own workers reach it through Do as before, and only the boundary that lets a
// stranger in has to ask.
func (s *Store) HandleInternal(verb string, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claim(verb)
	s.handlers[verb] = h
	s.private[verb] = true
}

// HandleInternalSpec is HandleInternal with a description.
//
// An internal callback is worth describing for the reader of the code rather
// than the writer of a script: it is the half of the surface that explains why
// a verb they can call answers when it does. It takes no example, because an
// example of a call the socket refuses would read as an invitation to make it.
func (s *Store) HandleInternalSpec(verb string, spec Spec, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claim(verb)
	s.handlers[verb] = h
	s.specs[verb] = spec
	s.private[verb] = true
}

// IsInternal reports whether the verb was registered as the application's own
// callback. An unregistered verb is not internal: it is unknown, which is a
// different answer and a different error.
func (s *Store) IsInternal(verb string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.private[verb]
}

// PublicVerbs is what a caller outside this process may use, sorted. It is what
// the socket answers with and what it will accept, so the two cannot disagree
// about which verbs exist.
func (s *Store) PublicVerbs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.handlers))
	for v := range s.handlers {
		if !s.private[v] {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// InternalVerbs is the other half, sorted, for the test that holds the split
// still.
func (s *Store) InternalVerbs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.private))
	for v := range s.private {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
