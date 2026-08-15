package engine

import (
	"fmt"
	"strings"
)

// Assertion is a machine-checkable claim about a run.
//
// What turns a scenario into a regression test. Without one, comparing two
// firmware versions means a human reading two event logs and forming an
// opinion; with one, a run passes or fails and a bisect can be automated.
type Assertion struct {
	// Kind is what is being claimed.
	Kind AssertKind
	// Node is who it is about; empty means any node.
	Node string
	// WithinMs bounds a timing claim.
	WithinMs uint32
	// AtLeast and AtMost bound a counting claim.
	AtLeast, AtMost int
	// MaxPct bounds a percentage claim, such as duty cycle.
	MaxPct float64
}

// AssertKind enumerates the claims worth making.
type AssertKind string

const (
	// AssertReceives is "node X receives something within N ms".
	AssertReceives AssertKind = "receives"
	// AssertDelivered is "at least N unique deliveries happened".
	AssertDelivered AssertKind = "delivered"
	// AssertDutyBelow is "no node exceeded N% duty cycle" — the compliance one,
	// and the one people most want to find out about before a deployment
	// rather than after a letter.
	AssertDutyBelow AssertKind = "duty-below"
	// AssertRelaysAtMost caps a node's transmissions, for catching a change
	// that turns a repeater into a shouter.
	AssertRelaysAtMost AssertKind = "relays-at-most"
)

// Result is one assertion's verdict, with the evidence for it.
type Result struct {
	Assertion Assertion
	Passed    bool
	// Detail is why, in words, and always populated — a failure with no
	// explanation sends someone back to the log this exists to replace.
	Detail string
}

func (a Assertion) String() string {
	switch a.Kind {
	case AssertReceives:
		return fmt.Sprintf("%s receives within %.1f s", a.Node, float64(a.WithinMs)/1000)
	case AssertDelivered:
		who := "unique deliveries"
		if a.Node != "" {
			who = a.Node + "'s unique deliveries"
		}
		// AtLeast unset (zero) picks the AtMost reading, because "at most 0" -
		// a containment bound, "nothing got out" - is a real claim a leakage
		// check needs to make, and would otherwise be indistinguishable from
		// AtMost never having been set at all. "at least 0" is never a claim
		// worth writing down: it holds trivially, so nothing is lost by
		// reading a zero AtLeast as "the AtMost field is the one that counts."
		if a.AtLeast == 0 {
			return fmt.Sprintf("at most %d %s", a.AtMost, who)
		}
		return fmt.Sprintf("at least %d %s", a.AtLeast, who)
	case AssertDutyBelow:
		return fmt.Sprintf("no node above %.2f%% duty cycle", a.MaxPct)
	case AssertRelaysAtMost:
		return fmt.Sprintf("%s transmits at most %d times", a.Node, a.AtMost)
	default:
		return string(a.Kind)
	}
}

// Check evaluates assertions against the run so far.
func (e *Engine) Check(assertions []Assertion) []Result {
	events := e.Events()
	board := e.Scoreboard()

	out := make([]Result, 0, len(assertions))
	for _, a := range assertions {
		out = append(out, e.checkOne(a, events, board))
	}
	return out
}

func (e *Engine) checkOne(a Assertion, events []Event, board []Score) Result {
	switch a.Kind {
	case AssertReceives:
		for _, ev := range events {
			if ev.Kind == "rx" && (a.Node == "" || ev.To == a.Node) {
				if ev.AtMs <= a.WithinMs {
					return Result{a, true, fmt.Sprintf("%s received at %.2f s", ev.To,
						float64(ev.AtMs)/1000)}
				}
			}
		}
		return Result{a, false, fmt.Sprintf("%s received nothing within %.1f s",
			nameOrAny(a.Node), float64(a.WithinMs)/1000)}

	case AssertDelivered:
		total := 0
		for _, s := range board {
			// An empty Node sums the whole board, same as before Node was
			// consulted here; a named one scopes the claim to that node alone -
			// the shape a leakage check needs ("did region B get anything at
			// all"), which AssertReceives already gets for free but this kind
			// did not.
			if a.Node == "" || s.Name == a.Node {
				total += s.UniqueDelivery
			}
		}
		// See String() above for why a zero AtLeast, not a positive AtMost,
		// is the signal that this assertion means a containment claim rather
		// than a reachability one - AtMost's own zero value is one this kind
		// legitimately needs to express.
		if a.AtLeast == 0 {
			return Result{a, total <= a.AtMost,
				fmt.Sprintf("%s: %d unique deliveries, wanted at most %d", nameOrAny(a.Node), total, a.AtMost)}
		}
		return Result{a, total >= a.AtLeast,
			fmt.Sprintf("%s: %d unique deliveries, wanted at least %d", nameOrAny(a.Node), total, a.AtLeast)}

	case AssertDutyBelow:
		worst, who := 0.0, ""
		for _, s := range board {
			if s.DutyCyclePct > worst {
				worst, who = s.DutyCyclePct, s.Name
			}
		}
		if worst <= a.MaxPct {
			return Result{a, true, fmt.Sprintf("worst was %s at %.2f%%", who, worst)}
		}
		return Result{a, false, fmt.Sprintf("%s reached %.2f%%, over the %.2f%% allowed",
			who, worst, a.MaxPct)}

	case AssertRelaysAtMost:
		for _, s := range board {
			if s.Name != a.Node {
				continue
			}
			return Result{a, s.Sent <= a.AtMost,
				fmt.Sprintf("%s transmitted %d times, allowed %d", s.Name, s.Sent, a.AtMost)}
		}
		return Result{a, false, fmt.Sprintf("no node named %q", a.Node)}
	}
	return Result{a, false, "unknown assertion"}
}

func nameOrAny(n string) string {
	if n == "" {
		return "any node"
	}
	return n
}

// Divergence is the first moment two runs stopped agreeing.
//
// The number that matters in an A/B: "delivered 1,204 versus 1,331" says
// something changed, and only the first divergence says where to look.
type Divergence struct {
	Found bool
	AtMs  uint32
	Node  string
	// A and B describe what each run did at that instant.
	A, B string
}

// Diverge compares two event ledgers and finds where they first differ.
//
// Compared on what each node did rather than on the raw sequence: two runs may
// interleave events between nodes differently while behaving identically, and a
// diff that flagged that would cry wolf on every comparison.
func Diverge(a, b []Event) Divergence {
	key := func(ev Event) string {
		return fmt.Sprintf("%s|%s|%s|%s", ev.Kind, ev.From, ev.To, shortDetail(ev.Detail))
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if key(a[i]) == key(b[i]) {
			continue
		}
		return Divergence{
			Found: true, AtMs: a[i].AtMs, Node: laneOf(a[i]),
			A: fmt.Sprintf("%s %s->%s %s", a[i].Kind, a[i].From, a[i].To, a[i].Detail),
			B: fmt.Sprintf("%s %s->%s %s", b[i].Kind, b[i].From, b[i].To, b[i].Detail),
		}
	}
	if len(a) != len(b) {
		longer, at := "A", uint32(0)
		if len(b) > len(a) {
			longer = "B"
			if len(a) < len(b) {
				at = b[len(a)].AtMs
			}
		} else if len(a) > 0 {
			at = a[len(b)].AtMs
		}
		return Divergence{Found: true, AtMs: at,
			A: fmt.Sprintf("%d events", len(a)), B: fmt.Sprintf("%d events", len(b)),
			Node: longer + " kept going"}
	}
	return Divergence{}
}

// shortDetail removes the numbers that legitimately differ run to run — SNR to
// a decimal place, airtime in milliseconds — so a divergence means a different
// *decision*, not a different rounding.
//
// Removed rather than truncated at: the numbers are in the middle of the
// sentence, and cutting at the first one throws away the words after it that
// say what happened.
func shortDetail(d string) string {
	var b strings.Builder
	for _, r := range d {
		if r < '0' || r > '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func laneOf(ev Event) string {
	if ev.Kind == "tx" {
		return ev.From
	}
	return ev.To
}
