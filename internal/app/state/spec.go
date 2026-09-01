package state

// What a verb is and what it takes, held as data beside the code that
// registers it.
//
// Every surface onto the verbs - the Go client, the Python client, the tools,
// the published reference - is a restatement of facts that live in the handler
// bodies, and each restatement is maintained by hand. That is how a surface can
// come to name a verb this tree does not register: nothing could compare the
// surface against anything, because there was nothing to compare it to.
//
// A parser over the handlers is not the answer either. What a verb takes is a
// decision, not a syntax, and it has to be written down by whoever decided it.
//
// It was written down inside the registration first, so that a reviewer would
// meet the description and the handler at once. That lost to what it produced.
// The descriptions are paragraphs, not labels: a three line registration became
// forty-five lines of concatenated prose with the handler it described as the
// last line, and reading a file of them told you nothing about what the code
// did. So they live in a sibling <basename>.verbs.json, one per file that
// registers verbs, and the types below are the schema those files are loaded
// and checked against. Nothing here is read while the workbench runs.

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
	// What the parameter is, and what an unusable value does. It renders as a
	// table cell in one document and as prose in another, and most of them run
	// to a sentence or three: a refusal a caller cannot predict is the part
	// worth writing down, and it does not fit in a label.
	What string `json:"what"`
}

// Example is one call, kept beside the verb it demonstrates.
//
// Held as data rather than in a document because an example is the part of a
// reference that rots first and says nothing when it does: a parameter renamed
// in the handler leaves the prose still reading plausibly. Beside the code, the
// test that runs it fails on the same commit that broke it.
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
	// What the verb is for, in the imperative. Not what its name already says:
	// a name cannot tell a caller that one verb answers four different ways
	// depending on what the session is doing, and that is the gap this closes.
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
