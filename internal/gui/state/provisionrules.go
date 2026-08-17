// The provisioning rule engine, as the panel reads and edits it.
//
// This package takes no dependency on internal/provision - state is
// deliberately a leaf with nothing but plain data in it, so these are the
// same shapes with the same field names, translated at the session boundary.
// A rule that means nothing without the firmware's own command table is not
// something the render loop should have an opinion about.
package state

// ProvisionKey is one row of the firmware's command table, as the panel needs
// it to build a chooser rather than a text box: its type, its legal values,
// and whether a companion can answer it at all.
type ProvisionKey struct {
	Name string
	// Kind is "string", "int", "bool" or "enum" - a label rather than an
	// enum-of-its-own, so this package still depends on nothing.
	Kind     string
	Enum     []string
	Min, Max int
	HasMin   bool
	HasMax   bool
	// Companion is true when this key has an equivalent over the app
	// protocol - loop.detect does not, and a control for it should say so on
	// a companion node rather than pretend.
	Companion bool
	Note      string
}

// ProvisionCondition is one clause of a rule's "when". An empty Field matches
// nothing - it is what a freshly added, not-yet-filled-in condition looks
// like, and the panel must not send it as though it meant "matches nothing"
// versus "not written yet" without checking.
type ProvisionCondition struct {
	Field, Op, Value string
	Custom           bool
	CustomGet        string
}

// ProvisionEffect is one clause of a rule's "then". Mode is "leave",
// "asimported", "add" (regions only) or "set".
type ProvisionEffect struct {
	Field, Mode, Value string
	CustomSet          string
}

// ProvisionRule is one when/then pair. A rule with no conditions matches
// every node - see ProvisionCondition's own doc - which is deliberately how
// "just set this on every node" is expressed: there is no separate concept
// of a base rule.
type ProvisionRule struct {
	Name       string
	Conditions []ProvisionCondition
	Effects    []ProvisionEffect
}

// ProvisionResolvedLine is one line of a resolved script - what a named node
// will actually be sent, and which rule asked for it. With more than one rule
// in play a node's script is no longer something an operator can work out by
// reading the panel, so this is what the preview shows instead.
type ProvisionResolvedLine struct {
	Command, RuleName string
}

// ProvisionResult is what one node said to its provisioning run.
type ProvisionResult struct {
	Node string
	Sent int
	// Refused is every reply that looked like a refusal, so the panel can
	// group nodes by identical outcome instead of forty rows all saying
	// "accepted".
	Refused []string
}
