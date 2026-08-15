package regression

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/engine"
)

func passResult(name string) CaseResult {
	return CaseResult{Case: Case{Name: name}, Verdict: Pass,
		Seeds: []SeedResult{{Seed: 1, Verdict: Pass}}}
}

func failResult(name string) CaseResult {
	a := engine.Assertion{Kind: engine.AssertDelivered, AtLeast: 100}
	r := engine.Result{Assertion: a, Passed: false, Detail: "60 unique deliveries, wanted at least 100"}
	return CaseResult{
		Case:    Case{Name: name, ExperimentID: "8f91c21a4b02e1a9"},
		Verdict: Fail,
		Seeds: []SeedResult{{
			Seed: 4417, Verdict: Fail,
			Checks: []CheckResult{{Result: r, Verdict: Fail}},
		}},
	}
}

func flagResult(name string) CaseResult {
	a := engine.Assertion{Kind: engine.AssertDelivered, AtLeast: 100}
	r := engine.Result{Assertion: a, Passed: false, Detail: "92 unique deliveries, wanted at least 100"}
	return CaseResult{
		Case:    Case{Name: name},
		Verdict: Flag,
		Seeds: []SeedResult{{
			Seed: 4417, Verdict: Flag,
			Checks: []CheckResult{{Result: r, Verdict: Flag}},
		}},
	}
}

// A clean run - every case passing - collapses to one line. A bot that
// writes an essay for a green run gets muted, which defeats the point of
// having it comment at all.
func TestACleanRunCollapsesToOneLine(t *testing.T) {
	body := Comment([]CaseResult{passResult("a"), passResult("b"), passResult("c")}, "repeater-v1.18.0")
	if !strings.Contains(body, Marker) {
		t.Error("the comment does not carry its own update marker")
	}
	if !strings.Contains(body, "3 scenarios passed") {
		t.Errorf("no clean summary line: %s", body)
	}
	if strings.Contains(body, "<details>") {
		t.Error("a clean run should not expand anything")
	}
}

// Only the diverging scenarios are expanded; the rest are a count, not a
// list - "a bot that writes an essay gets muted."
func TestOnlyDivergingScenariosAreExpanded(t *testing.T) {
	results := []CaseResult{
		passResult("fine-1"), passResult("fine-2"),
		failResult("broken-flood"), flagResult("noisy-delivery"),
	}
	body := Comment(results, "repeater-v1.18.0")

	if strings.Contains(body, "fine-1") || strings.Contains(body, "fine-2") {
		t.Error("a passing scenario was named in the comment body")
	}
	if !strings.Contains(body, "broken-flood") {
		t.Error("the failing scenario is missing")
	}
	if !strings.Contains(body, "noisy-delivery") {
		t.Error("the flagged scenario is missing")
	}
	if !strings.Contains(body, "2 other scenarios passed") {
		t.Errorf("the passing count is missing or wrong: %s", body)
	}
	// Worst first: the regression before the flag.
	if strings.Index(body, "broken-flood") > strings.Index(body, "noisy-delivery") {
		t.Error("the flagged scenario was listed before the failing one")
	}
}

// The reproduce line names something the reader can actually run, and
// carries the experiment ID when the case has one - "the author can
// reproduce locally from the comment alone."
func TestReproduceLineIsActionable(t *testing.T) {
	body := Comment([]CaseResult{failResult("broken-flood")}, "")
	if !strings.Contains(body, "meshcoresim verify broken-flood.json") {
		t.Errorf("no reproduce command: %s", body)
	}
	if !strings.Contains(body, "8f91c21a4b02e1a9") {
		t.Errorf("the experiment id is missing from the reproduce line: %s", body)
	}
}

// A run mixing failures and flags reports both counts distinctly - a
// flagged case is not the same claim as a failed one, and folding them
// together would hide exactly the distinction plan 6 exists to draw.
func TestFailedAndFlaggedAreCountedSeparately(t *testing.T) {
	body := Comment([]CaseResult{failResult("a"), flagResult("b"), passResult("c")}, "")
	if !strings.Contains(body, "1 regression") {
		t.Errorf("regression count missing: %s", body)
	}
	if !strings.Contains(body, "1 flagged") {
		t.Errorf("flagged count missing: %s", body)
	}
}
