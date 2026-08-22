// What a run is told to send, and what has to be true of it afterwards.
//
// These were reachable only through Call("schedule.add", map[string]any{
// "at_ms": 5000, "every_ms": 20000}), which is the shape this package exists
// to remove: a verb name spelled by hand, parameters in milliseconds because
// that is what the wire happens to use, and nothing a compiler can check. An
// example written that way is an advertisement for not using the library.
package client

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"strings"
	"time"
)

// Schedule is what the mesh is told to send, and when. Live.
type Schedule struct{ w *Workbench }

// Schedule reaches the fixture's traffic.
func (w *Workbench) Schedule() Schedule { return Schedule{w} }

// Send is one scheduled line at a node.
//
// At and Every are durations of the mesh's own clock, not yours. The verb
// underneath takes milliseconds; nobody writing a script should have to.
type Send struct {
	Node    string
	Command string
	At      time.Duration
	// Every repeats it. Zero sends once.
	Every time.Duration
}

// Add schedules one.
//
// Repeating traffic has worked all along and nothing said so, which to
// somebody writing a script is the same as it not existing.
func (s Schedule) Add(ctx context.Context, send Send) error {
	p := map[string]any{"node": send.Node, "command": send.Command}
	if send.At > 0 {
		p["at_ms"] = float64(send.At.Milliseconds())
	}
	if send.Every > 0 {
		p["every_ms"] = float64(send.Every.Milliseconds())
	}
	return s.w.Do(ctx, "schedule.add", p)
}

// Clear forgets all of them.
func (s Schedule) Clear(ctx context.Context) error {
	return s.w.Do(ctx, "schedule.clear", nil)
}

// Check is one assertion, and what the run made of it.
type Check struct {
	Kind   string `json:"kind"`
	Node   string `json:"node"`
	Passed bool   `json:"pass"`
	Got    string `json:"got"`
	Want   string `json:"want"`
}

func (c Check) String() string {
	mark := "pass"
	if !c.Passed {
		mark = "FAIL"
	}
	where := ""
	if c.Node != "" {
		where = " at " + c.Node
	}
	return fmt.Sprintf("%s  %s%s: got %s, want %s", mark, c.Kind, where, c.Got, c.Want)
}

// Report is what a run passed and failed, with what it was measured under.
type Report struct {
	Passed int     `json:"passed"`
	Total  int     `json:"total"`
	Checks []Check `json:"results"`
	// Provenance is what the numbers were measured under, carried with the
	// verdict because a delivery figure without it is the number this project
	// exists not to publish.
	Provenance Provenance `json:"-"`
}

// OK reports whether every assertion held.
//
// A report with no assertions is not OK. A fixture that carries none can
// report but cannot pass, and a green tick that checked nothing is the worst
// outcome available here.
func (r Report) OK() bool { return r.Total > 0 && r.Passed == r.Total }

// Failures are the ones that did not hold.
func (r Report) Failures() []Check {
	var out []Check
	for _, c := range r.Checks {
		if !c.Passed {
			out = append(out, c)
		}
	}
	return out
}

func (r Report) String() string {
	lines := []string{r.Provenance.String()}
	if r.Total == 0 {
		lines = append(lines, "no assertions, so this run checked nothing")
	} else {
		lines = append(lines, fmt.Sprintf("%d of %d assertions passed", r.Passed, r.Total))
	}
	for _, c := range r.Failures() {
		lines = append(lines, "  "+c.String())
	}
	return strings.Join(lines, "\n")
}

// WriteJUnit writes a JUnit file, with the caveats inside it.
//
// In the file rather than only on stdout, because the file is what a CI system
// keeps and shows six months later - and a delivery figure with no note of
// what the model assumed is exactly the number this project exists not to
// publish.
func (r Report) WriteJUnit(path, suite string) error {
	if suite == "" {
		suite = "meshbench"
	}
	type failure struct {
		XMLName xml.Name `xml:"failure"`
		Message string   `xml:"message,attr"`
	}
	type testcase struct {
		XMLName   xml.Name `xml:"testcase"`
		Classname string   `xml:"classname,attr"`
		Name      string   `xml:"name,attr"`
		Failure   *failure `xml:",omitempty"`
	}
	type property struct {
		XMLName xml.Name `xml:"property"`
		Name    string   `xml:"name,attr"`
		Value   string   `xml:"value,attr"`
	}
	type suiteXML struct {
		XMLName    xml.Name   `xml:"testsuite"`
		Name       string     `xml:"name,attr"`
		Tests      int        `xml:"tests,attr"`
		Failures   int        `xml:"failures,attr"`
		Properties []property `xml:"properties>property"`
		Cases      []testcase `xml:",omitempty"`
	}
	out := suiteXML{
		Name: suite, Tests: r.Total, Failures: len(r.Failures()),
		Properties: []property{{
			Name: "meshbench.provenance", Value: r.Provenance.String(),
		}},
	}
	for _, c := range r.Checks {
		name := c.Kind
		if c.Node != "" {
			name += " at " + c.Node
		}
		tc := testcase{Classname: suite + ".assertions", Name: name}
		if !c.Passed {
			tc.Failure = &failure{Message: fmt.Sprintf("got %s, want %s", c.Got, c.Want)}
		}
		out.Cases = append(out.Cases, tc)
	}
	body, err := xml.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path,
		append([]byte(xml.Header), body...), 0o644) //nolint:gosec // a report, not a secret
}

// Assertions are what has to be true for a run to have passed. Live.
type Assertions struct{ w *Workbench }

// Assertions reaches them.
func (w *Workbench) Assertions() Assertions { return Assertions{w} }

// Assertion is one claim, in the general form.
type Assertion struct {
	// Kind is what is being counted. AssertDelivered and AssertSent are the
	// ones this build understands; one it does not is a failure rather than a
	// pass, because a green run that checked nothing is the worst outcome
	// available here.
	Kind    string
	Node    string
	AtLeast int
	AtMost  int
	MaxPct  float64
	Within  time.Duration
}

// The assertion kinds this build understands.
const (
	AssertDelivered = "delivered"
	AssertSent      = "sent"
)

// Add records one.
func (a Assertions) Add(ctx context.Context, want Assertion) error {
	p := map[string]any{"kind": want.Kind}
	if want.Node != "" {
		p["node"] = want.Node
	}
	if want.AtLeast != 0 {
		p["at_least"] = float64(want.AtLeast)
	}
	if want.AtMost != 0 {
		p["at_most"] = float64(want.AtMost)
	}
	if want.MaxPct != 0 {
		p["max_pct"] = want.MaxPct
	}
	if want.Within > 0 {
		p["within_ms"] = float64(want.Within.Milliseconds())
	}
	return a.w.Do(ctx, "assert.add", p)
}

// Delivered is the common one: at least this many nodes received something.
func (a Assertions) Delivered(ctx context.Context, atLeast int) error {
	return a.Add(ctx, Assertion{Kind: AssertDelivered, AtLeast: atLeast})
}

// Sent bounds what a node - or the whole mesh - transmitted.
//
// AtMost is the interesting one: it is how a relay-suppression change is held
// to not having made the mesh chattier.
func (a Assertions) Sent(ctx context.Context, node string, atLeast, atMost int) error {
	return a.Add(ctx, Assertion{
		Kind: AssertSent, Node: node, AtLeast: atLeast, AtMost: atMost})
}

// Check measures every assertion against the run so far.
func (a Assertions) Check(ctx context.Context) (Report, error) {
	var r Report
	if err := a.w.CallInto(ctx, "assert.check", nil, &r); err != nil {
		return r, err
	}
	p, err := a.w.Provenance(ctx)
	if err != nil {
		return r, err
	}
	r.Provenance = p
	return r, nil
}
