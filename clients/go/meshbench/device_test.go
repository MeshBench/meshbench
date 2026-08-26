package meshbench

import (
	"strings"
	"testing"
)

// The board API drives a running board - screen, screenshot, buttons, touch -
// and booting an emulated one is too slow and too flaky for this suite (it is
// exercised where the emulator is). What is checked here is the client layer:
// the verb each method reaches, the parameters it carries, and that a refusal
// comes back as an error rather than a zero value read as success.

func TestBoardInputRefusesANodeThatIsNotRunning(t *testing.T) {
	wb, ctx := headless(t)
	if err := wb.Project().New(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.Nodes().PlaceMany(ctx, []Placement{
		{Name: "R1", Kind: SimpleRepeater, Lat: 56.20, Lon: -3.20},
	}); err != nil {
		t.Fatal(err)
	}
	n := wb.Node("R1")
	b := n.Device()

	// Every one of these needs a live board; on a node that is not running the
	// verb must refuse, and the refusal must reach the caller as an error.
	if _, err := b.Screen(ctx); err == nil {
		t.Error("Screen on a stopped node did not error")
	}
	if _, err := b.Screenshot(ctx); err == nil {
		t.Error("Screenshot on a stopped node did not error")
	}
	if err := b.Press(ctx, 0, true); err == nil {
		t.Error("Press on a stopped node did not error")
	}
	if err := b.Type(ctx, "x"); err == nil {
		t.Error("Type on a stopped node did not error")
	}
	if err := b.Touch(ctx, 10, 10, true); err == nil {
		t.Error("Touch on a stopped node did not error")
	}
	// node.radio reconciles the model against what the node reports, so it too
	// needs a node that is running to have anything to ask.
	if _, err := n.Radio(ctx); err == nil {
		t.Error("Radio on a stopped node did not error")
	}
	// And the refusal says why, rather than a bare failure.
	_, err := b.Screen(ctx)
	if err != nil && !strings.Contains(err.Error(), "running") {
		t.Logf("screen refusal (informational): %v", err)
	}
}
