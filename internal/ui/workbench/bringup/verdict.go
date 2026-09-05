// What a row says about itself.
//
// A table of facts about a board is a list nobody reads. The verdict is what
// makes it a diagnosis: every row states whether the thing it names is
// behaving, and the two interesting answers are the ones that are about an
// absence - a line nothing has happened on, and a part we have no model for.
//
// The distinction between those two is the one this package exists to keep. A
// lamp is drawn as an outline rather than as an unlit lamp because "off" and
// "not modelled" are different facts and the second one is about us; the same
// rule decides every verdict here, and a row we cannot answer for says so
// rather than reporting a zero somebody would believe.
package bringup

import (
	"image/color"

	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// Verdict is what the interface has concluded about one declared thing.
type Verdict int

const (
	// Agrees: the board declared it and the firmware is using it as declared.
	Agrees Verdict = iota
	// Diverged: the firmware left it as something the board did not declare.
	// The expensive one, and the only one that is a bug on sight.
	Diverged
	// Silent: declared, and nothing has come back from it. Not proof of a
	// fault - a button nobody pressed is silent and correct - so it is drawn
	// as a caution rather than a failure, and the row says what it would take
	// to tell the two apart.
	Silent
	// NotModelled: we have no model for this part, so there is nothing to
	// observe and never will be until somebody writes one. About us, not the
	// board.
	NotModelled
	// Undeclared: nothing observable is claimed for it yet. The honest answer
	// for a pin whose direction and level nobody has instrumented, and the
	// reason the wiring table does not print a confident dash.
	Undeclared
)

func (v Verdict) String() string {
	switch v {
	case Diverged:
		return "diverged"
	case Silent:
		return "silent"
	case NotModelled:
		return "not modelled"
	case Undeclared:
		// Nothing to conclude. The Observed column already says the fact -
		// that nothing instruments this line - and repeating it here put the
		// same sentence in two columns of every row.
		return ""
	}
	return "agrees"
}

// Colour is the state this verdict is, which comes from the theme's semantic
// three and never from a literal.
func (v Verdict) Colour(t *theme.Theme) color.NRGBA {
	switch v {
	case Diverged:
		return t.P.Bad
	case Silent:
		return t.P.Warn
	case NotModelled, Undeclared:
		return t.P.Faint
	}
	return t.P.Good
}

// Counts is how many of each a table came to, for the status bar.
//
// The bar says the two that matter and stays quiet about the rest: a count of
// things that agree is a number nobody acts on.
type Counts struct {
	Total, Diverged, Silent, NotModelled int
}

func (c *Counts) add(v Verdict) {
	c.Total++
	switch v {
	case Diverged:
		c.Diverged++
	case Silent:
		c.Silent++
	case NotModelled:
		c.NotModelled++
	}
}
