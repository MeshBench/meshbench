package meshbench

import (
	"os"
	"strings"
	"testing"
	"time"
)

// The generated sets, held to what the workbench actually accepts.
//
// A constant that compiles and is not a board the simulator knows is worse
// than a string, because it looks checked. So every one of them is offered to
// a real session, and the count is compared with what the workbench lists.

func TestEveryGeneratedBoardIsOneTheWorkbenchKnows(t *testing.T) {
	wb, ctx := headless(t)
	if err := wb.Project().New(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if len(Boards) == 0 {
		t.Fatal("the generated board list is empty")
	}
	for i, b := range Boards {
		name := "n" + strings.ReplaceAll(string(b), "-", "")
		if _, err := wb.Nodes().Place(ctx, Placement{
			Name: name, Lat: 56 + float64(i)/1000, Lon: -3, Board: b,
		}); err != nil {
			t.Errorf("%s is in the generated list and the workbench refused it: %v", b, err)
		}
	}
}

func TestEveryGeneratedKindIsOneTheWorkbenchKnows(t *testing.T) {
	wb, ctx := headless(t)
	if err := wb.Project().New(ctx, ""); err != nil {
		t.Fatal(err)
	}
	for i, k := range []Kind{
		SimpleRepeater, AdvancedRepeater, Companion,
		RoomServer, SDRObserver, Emitter,
	} {
		if _, err := wb.Nodes().Place(ctx, Placement{
			Name: string(k), Kind: k, Lat: 56 + float64(i)/1000, Lon: -3,
		}); err != nil {
			t.Errorf("kind %s was refused: %v", k, err)
		}
	}
}

func TestEveryGeneratedPresetIsOneTheWorkbenchKnows(t *testing.T) {
	wb, ctx := headless(t, Fixture("fife-strict"))
	if len(Presets) == 0 {
		t.Fatal("the generated preset list is empty")
	}
	for _, p := range Presets {
		if err := wb.Do(ctx, "radio.preset", map[string]any{
			"preset": string(p), "node": "Abernethy Repeater",
		}); err != nil {
			t.Errorf("preset %s was refused: %v", p, err)
		}
	}
}

// Scheduling and asserting, through the shape rather than through Call.
//
// The verb takes milliseconds; a caller says twenty seconds. That difference
// is the entire reason this layer exists.
func TestSchedulingAndAsserting(t *testing.T) {
	// A blank network rather than a fixture: the shipped ones carry their own
	// assertions, and counting mine among theirs would prove nothing.
	wb, ctx := headless(t)
	if err := wb.Project().New(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.Nodes().Place(ctx, Placement{
		Name: "R1", Lat: 56.2, Lon: -3.2}); err != nil {
		t.Fatal(err)
	}

	if err := wb.Schedule().Add(ctx, Send{
		Node: "R1", Command: "public hello",
		At: 5 * time.Second, Every: 20 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	snap, err := wb.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := snap["scheduled_sends"].(float64); got != 1 {
		t.Fatalf("%v scheduled sends, want 1", snap["scheduled_sends"])
	}

	if err := wb.Assertions().Delivered(ctx, 1); err != nil {
		t.Fatal(err)
	}
	report, err := wb.Assertions().Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 1 {
		t.Fatalf("checked %d assertions, want 1", report.Total)
	}
	// Nothing has run, so it has not been met - and the report says which one
	// and what it saw, which is the point of reporting per assertion.
	if report.OK() {
		t.Error("an unmet assertion reported OK")
	}
	if len(report.Failures()) != 1 || report.Failures()[0].Kind == "" {
		t.Errorf("the failure does not name itself: %+v", report.Failures())
	}
	// And the caveats travel with the verdict.
	if !strings.Contains(report.String(), "best case") {
		t.Errorf("the report does not carry its provenance:\n%s", report)
	}
}

// A report with no assertions is not a pass. A green tick that checked nothing
// is the worst outcome available here.
func TestNoAssertionsIsNotAPass(t *testing.T) {
	var empty Report
	if empty.OK() {
		t.Error("a report with no assertions reported OK")
	}
	if !strings.Contains(empty.String(), "checked nothing") {
		t.Errorf("it does not say so: %s", empty)
	}
}

func TestJUnitCarriesTheProvenance(t *testing.T) {
	path := t.TempDir() + "/results.xml"
	r := Report{
		Passed: 1, Total: 2,
		Checks: []Check{
			{Kind: "delivered", Passed: true, Got: "40", Want: "at least 40"},
			{Kind: "sent", Node: "R1", Got: "99", Want: "at most 12"},
		},
		Provenance: Provenance{RFMode: "waveform", Seed: 9001},
	}
	if err := r.WriteJUnit(path, ""); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{"best case", "waveform", "at most 12", `failures="1"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the JUnit file does not contain %q:\n%s", want, body)
		}
	}
}
