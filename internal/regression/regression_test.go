package regression

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/fixture"
)

func TestASavedCaseRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "case.json")
	c := NewCase("agc-gain-does-not-revert", "fixture-fife-strict", []uint64{4417, 9001},
		"repeater-v1.17.0", "companion-v1.17.0", 90_000, nil,
		[]fixture.Assertion{{Kind: "delivered", AtLeast: 10}}, 4.2, "8f91c21a4b02e1a9")
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCase(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != c.Name || got.Fixture != c.Fixture || len(got.Seeds) != 2 {
		t.Fatalf("did not round-trip: %+v", got)
	}
	if got.RepeaterVersion != "repeater-v1.17.0" || got.CompanionVersion != "companion-v1.17.0" {
		t.Fatalf("firmware pins did not round-trip: %+v", got)
	}
	if got.ToleranceBandPct != 4.2 {
		t.Fatalf("tolerance band did not round-trip: got %v", got.ToleranceBandPct)
	}
	if got.ExperimentID != "8f91c21a4b02e1a9" {
		t.Fatalf("experiment id did not round-trip: got %q", got.ExperimentID)
	}
}

// A malformed scenario is refused with a clear complaint rather than
// silently skipped - the reader of a directory report needs to know a case
// did not run, not infer it from an absent row.
func TestAMalformedCaseIsRefusedWithAComplaint(t *testing.T) {
	dir := t.TempDir()

	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	cases := []struct {
		name, file, content, wantIn string
	}{
		{"not json", "bad.json", "{not json", "not a regression case"},
		{"no version", "noversion.json", `{"fixture":"x","seeds":[1]}`, "no format_version"},
		{"future version", "future.json",
			`{"format_version":99,"fixture":"x","seeds":[1]}`, "format version 99"},
		{"no fixture", "nofixture.json", `{"format_version":1,"seeds":[1]}`, "names no fixture"},
		{"no seeds", "noseeds.json", `{"format_version":1,"fixture":"x"}`, "names no seeds"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := write(c.file, c.content)
			_, err := LoadCase(path)
			if err == nil {
				t.Fatal("a malformed case loaded without complaint")
			}
			if !strings.Contains(err.Error(), c.wantIn) {
				t.Fatalf("complaint does not say what is wrong: %v", err)
			}
		})
	}
}

// A well-formed case with a sensible default name (from its filename, when
// the file itself does not carry one) loads cleanly.
func TestACaseWithNoNameTakesItsFilename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "the-loop-no-storm.json")
	body, _ := json.Marshal(map[string]any{
		"format_version": 1, "fixture": "x", "seeds": []int{1},
	})
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadCase(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "the-loop-no-storm" {
		t.Errorf("name: got %q want %q", c.Name, "the-loop-no-storm")
	}
}

// Verdicts rank pass < flag < fail, and a case's overall verdict is the
// worst among its seeds - one broken seed makes the case the broken one.
func TestWorseOrdering(t *testing.T) {
	if worse(Fail, Flag) {
		t.Error("flag ranked worse than fail")
	}
	if worse(Flag, Pass) {
		t.Error("pass ranked worse than flag")
	}
	if !worse(Pass, Flag) {
		t.Error("flag did not rank worse than pass")
	}
	if !worse(Flag, Fail) {
		t.Error("fail did not rank worse than flag")
	}
}

// A stochastic AssertDelivered miss within the tolerance band flags rather
// than fails - the whole point of a band, and the risk plan §6 calls out
// explicitly: too tight a band on a hard fail makes a flaky gate that gets
// ignored.
func TestToleranceBandTurnsAMissIntoAFlagNotAFail(t *testing.T) {
	assertion := engine.Assertion{Kind: engine.AssertDelivered, AtLeast: 100}

	// 8% short - inside a 10% band.
	inBand := engine.Result{Assertion: assertion, Passed: false,
		Detail: "92 unique deliveries, wanted at least 100"}
	// 40% short - well outside a 10% band, a real regression.
	outOfBand := engine.Result{Assertion: assertion, Passed: false,
		Detail: "60 unique deliveries, wanted at least 100"}

	if v := classifyVerdict(inBand, 10); v != Flag {
		t.Errorf("a miss inside the band: got %v want flag", v)
	}
	if v := classifyVerdict(outOfBand, 10); v != Fail {
		t.Errorf("a miss outside the band: got %v want fail", v)
	}
	if v := classifyVerdict(inBand, 0); v != Fail {
		t.Errorf("no band at all: got %v want fail (an invariant, not softened)", v)
	}
}

// A hard assertion kind - not AssertDelivered - never flags, no matter the
// band: a duty-cycle violation is an invariant, not a stochastic metric.
func TestHardAssertionKindsNeverFlag(t *testing.T) {
	r := engine.Result{
		Assertion: engine.Assertion{Kind: engine.AssertDutyBelow, MaxPct: 10},
		Passed:    false, Detail: "worst was x at 15.00%",
	}
	if v := classifyVerdict(r, 50); v != Fail {
		t.Errorf("a duty-cycle violation with a 50%% band: got %v want fail", v)
	}
}

func TestDeliveredFromParsesTheMeasuredCount(t *testing.T) {
	cases := map[string]int{
		"92 unique deliveries, wanted at least 100": 92,
		"0 unique deliveries, wanted at least 10":   0,
		"not a detail string at all":                0,
	}
	for detail, want := range cases {
		if got := deliveredFrom(detail); got != want {
			t.Errorf("deliveredFrom(%q) = %d, want %d", detail, got, want)
		}
	}
}
