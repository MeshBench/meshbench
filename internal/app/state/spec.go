package state

import "sort"

// What a verb is and what it takes, said where it is registered.
//
// Every surface onto the verbs - the Go client, the Python client, the
// tools, the published reference - is a restatement of facts that live in the
// handler bodies, and each restatement is maintained by hand. That is how a
// surface can come to name a verb this tree does not register: nothing could
// compare the surface against anything, because there was nothing to compare
// it to.
//
// A parser over the handlers is not the answer either, and the tree already
// has one: tools/verbdoc reads them with a brace matcher and regular
// expressions, and says of itself that 77 verbs read their parameters in ways
// nothing outside the handler can see. What a verb takes is a decision, not a
// syntax, and it has to be written down by whoever decided it.
//
// So it goes at the registration, next to the code it describes, where a
// reviewer sees both at once and a new verb cannot be added without passing
// the place the description belongs.

// ParamType is the JSON shape a parameter arrives as.
//
// Deliberately not a schema language. Type and required-ness are enough for a
// typed binding and for an error a caller can act on; a schema is a second
// thing to keep correct, and this exists because the first thing was not.
type ParamType string

const (
	ParamString ParamType = "string"
	ParamNumber ParamType = "number"
	ParamBool   ParamType = "bool"
	ParamObject ParamType = "object"
	ParamArray  ParamType = "array"
)

// Param is one parameter a verb reads.
type Param struct {
	Name string    `json:"name"`
	Type ParamType `json:"type"`
	// Required says the verb refuses without it. A verb that accepts a bare
	// value rather than an object marks that parameter Primary.
	Required bool `json:"required,omitempty"`
	// Primary marks the one parameter a bare string or number may mean, which
	// is at most one per verb. It is the same rule the handlers follow, where
	// stringField reads the primary and namedField reads everything else.
	Primary bool `json:"primary,omitempty"`
	// What is one line. Not a paragraph: this is read in a table.
	What string `json:"what"`
}

// Example is one call, kept beside the verb it demonstrates.
//
// Held here rather than in a document because an example is the part of a
// reference that rots first and says nothing when it does: a parameter renamed
// in the handler leaves the prose still reading plausibly. Beside the
// registration, the test that runs it fails on the same commit that broke it.
type Example struct {
	// Params exactly as they arrive: an object for the usual form, or a bare
	// value for a verb that accepts one in place of its primary parameter.
	Params any `json:"params"`
	// What this particular call is for. A few words, because the verb's own
	// description has already said what the verb is.
	What string `json:"what,omitempty"`
	// Runnable says a test may make this call against a headless session
	// holding two nodes and nothing else, and expect it to be answered. Left
	// false where the call needs a window, a network, a firmware build or a
	// running clock, which cannot be stood up in a unit test; those examples
	// are still checked against the parameters the verb declares.
	Runnable bool `json:"runnable,omitempty"`
}

// Spec is a verb's description, its parameters and a call that exercises it.
type Spec struct {
	// What the verb does, in one line, in the imperative.
	What string `json:"what"`
	// Params in the order a reader should meet them, primary first.
	Params []Param `json:"params,omitempty"`
	// Returns names the keys of the object the verb answers with, or is empty
	// where it answers with something that is not an object.
	Returns []string `json:"returns,omitempty"`
	// Answers is the shape in a sentence, for the verbs whose return is not
	// self-evident from its keys: a count beside its rows, a list rather than
	// an object, nothing at all.
	Answers string `json:"answers,omitempty"`
	// Example is the call the reference prints. Every public verb that
	// describes itself carries one.
	Example *Example `json:"example,omitempty"`
}

// described reports whether anything was said at all, which is what the parity
// test counts.
func (s Spec) described() bool { return s.What != "" }

// HandleSpec registers a verb and says what it is.
//
// Use this rather than Handle. Handle remains for tests, which register a
// stub to see what calls it and have nothing to describe.
func (s *Store) HandleSpec(verb string, spec Spec, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claim(verb)
	s.handlers[verb] = h
	s.specs[verb] = spec
}

// Specs is what every described verb says about itself, for the manifest and
// the parity test that keeps it honest.
func (s *Store) Specs() map[string]Spec {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]Spec, len(s.specs))
	for v, sp := range s.specs {
		if sp.described() {
			out[v] = sp
		}
	}
	return out
}

// Undescribed lists the registered verbs that say nothing about themselves,
// sorted. Empty is the goal.
func (s *Store) Undescribed() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for v := range s.handlers {
		if !s.specs[v].described() {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
