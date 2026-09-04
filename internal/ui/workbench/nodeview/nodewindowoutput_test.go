package nodeview

import (
	"slices"
	"testing"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
	"github.com/MeshBench/meshbench/internal/ui/uitest"
)

// Every node that runs firmware grows an Output tab, whatever it runs and
// wherever it runs it.
//
// The question the tab answers - what did this thing print - is the same for a
// board under an emulator and a build on this machine. Only an observer is
// exempt, because an observer runs no firmware and has nothing to have said.
func TestEveryFirmwareNodeGetsTheOutputTab(t *testing.T) {
	for _, kind := range []string{"companion", "simple-repeater", "room-server"} {
		p := &WindowPanel{}
		p.Kind = kind
		if !slices.Contains(p.visibleTabs(), TabOutput) {
			t.Errorf("a %s has no Output tab, so what it printed is unreachable", kind)
		}
	}
	p := &WindowPanel{}
	p.Kind = "sdr-observer"
	if slices.Contains(p.visibleTabs(), TabOutput) {
		t.Error("an SDR observer offers an Output tab; it runs no firmware")
	}
}

// The pane shows this node's output and no other's.
//
// The snapshot carries a pane per node and source, and a pane draws its own.
// A pane that drew whatever the snapshot held would show one board's boot
// chain under the other board's name, which is worse than an empty pane: it is
// a wrong answer to the question the pane exists for.
func TestThePaneRefusesAnotherNodesOutput(t *testing.T) {
	o := &outputPane{}
	o.build()
	s := &state.Snapshot{Outputs: []state.OutputPane{{
		Node: "GB7AAA", Source: "serial", Total: 2,
		Lines: []string{"ets Jul 29 2019", "boot ok"},
	}}}
	if lines, _, _, _ := o.readFrom("GB7BBB", s); len(lines) != 0 {
		t.Errorf("a window on GB7BBB drew GB7AAA's output: %q", lines)
	}
	lines, total, _, _ := o.readFrom("GB7AAA", s)
	if len(lines) != 2 || total != 2 {
		t.Errorf("the node's own output did not reach it: %d lines, total %d", len(lines), total)
	}
	// And not another source's, either: switching to the emulator log and
	// being shown the serial one is the same fault wearing a different hat.
	o.source = "emulator"
	if lines, _, _, _ := o.readFrom("GB7AAA", s); len(lines) != 0 {
		t.Errorf("the emulator pane drew the serial log: %q", lines)
	}
}

// Pausing stops the pane chasing the end of the file.
//
// The audit excuses this button from reaching a verb, because following is a
// property of the pane rather than of the session. This is the test that holds
// the claim the excuse rests on.
func TestPausingStopsTheOutputPaneChasingTheEnd(t *testing.T) {
	p := &WindowPanel{Node: "GB7XYZ"}
	p.out.build()
	if !p.out.follow {
		t.Fatal("the pane does not follow by default; the newest line is the one being waited for")
	}
	h := uitest.New(func(_ *theme.Theme, gtx layout.Context, _ *state.Snapshot) layout.Dimensions {
		p.outputClicks(gtx)
		return layout.Dimensions{}
	}, nil)
	p.out.pauseBtn.Click.Click()
	h.Frame()
	if p.out.follow {
		t.Error("pressing pause left the pane still chasing the end of the file")
	}
	if p.out.pauseBtn.Label != "follow" {
		t.Errorf("the button still says %q, so nothing says how to resume", p.out.pauseBtn.Label)
	}
	p.out.pauseBtn.Click.Click()
	h.Frame()
	if !p.out.follow {
		t.Error("pressing follow did not resume")
	}
}

// Choosing a source asks the session for that source, and choosing the one
// already showing asks again.
//
// The second half is the useful one: a stopped node's file is not refreshed by
// the tick, so pressing the source you are on is how somebody asks again. A
// button that does nothing there is a dead button on exactly the board that
// most needs looking at.
func TestChoosingASourceAsksForIt(t *testing.T) {
	p := &WindowPanel{Node: "GB7XYZ"}
	var asked []string
	p.OnDo = func(verb string, params any) {
		if verb != "node.output" {
			return
		}
		m, _ := params.(map[string]any)
		asked = append(asked, m["source"].(string))
	}
	p.out.build()

	p.askOutput(p.out.source)
	if len(asked) != 1 || asked[0] != "serial" {
		t.Fatalf("the pane asked %v, want one ask for serial", asked)
	}
	// Drawing again does not ask again: the tick refreshes what is showing.
	p.askOutput(p.out.source)
	if len(asked) != 1 {
		t.Errorf("the pane asked %d times for the same source", len(asked))
	}

	h := uitest.New(func(_ *theme.Theme, gtx layout.Context, _ *state.Snapshot) layout.Dimensions {
		p.outputClicks(gtx)
		p.askOutput(p.out.source)
		return layout.Dimensions{}
	}, nil)
	emulator := 0
	for i := range outputSources {
		if outputSources[i].key == "emulator" {
			emulator = i
		}
	}
	p.out.srcBtns[emulator].Click.Click()
	h.Frame()
	if got := asked[len(asked)-1]; got != "emulator" {
		t.Errorf("choosing the emulator asked for %q", got)
	}
	before := len(asked)
	p.out.srcBtns[emulator].Click.Click() // the same one again
	h.Frame()
	if len(asked) == before {
		t.Error("pressing the source already showing asked for nothing, so it cannot be re-read")
	}
}

// Two windows on two boards, and two logs of one board, all stay filled.
//
// One slot for all of them was what the world had: whichever pane asked last
// won and the others drew empty until their turn came round again, which reads
// as the workbench losing the log. Nothing on disk was ever lost.
func TestEveryOpenPaneKeepsItsOwnLog(t *testing.T) {
	s := &state.Snapshot{Outputs: []state.OutputPane{
		{Node: "GB7AAA", Source: "serial", Total: 1, Lines: []string{"A serial"}},
		{Node: "GB7AAA", Source: "emulator", Total: 1, Lines: []string{"A emulator"}},
		{Node: "GB7BBB", Source: "serial", Total: 1, Lines: []string{"B serial"}},
	}}
	for _, want := range []struct{ node, source, line string }{
		{"GB7AAA", "serial", "A serial"},
		{"GB7AAA", "emulator", "A emulator"},
		{"GB7BBB", "serial", "B serial"},
	} {
		o := &outputPane{}
		o.build()
		o.source = want.source
		lines, _, _, _ := o.readFrom(want.node, s)
		if len(lines) != 1 || lines[0] != want.line {
			t.Errorf("%s/%s drew %q, want %q", want.node, want.source, lines, want.line)
		}
	}
	// And the Hardware tab's strip reads the serial pane of its own node,
	// not whichever one happens to be first.
	if got := outputSummary("GB7BBB", s, 4); len(got) != 1 || got[0] != "B serial" {
		t.Errorf("the strip drew %q", got)
	}
}

// Switching source does not blank the pane it switched away from.
//
// The subscription is what changes; the pane the window came from stays in the
// world and stays refreshed, so switching back shows the log rather than
// "nothing on this source yet" until the next tick.
func TestSwitchingSourceLeavesTheOtherLogInPlace(t *testing.T) {
	var asked []map[string]any
	do := func(_ string, params any) {
		asked = append(asked, params.(map[string]any))
	}
	var last string
	askOutputOnce(do, &last, "GB7AAA", "serial")
	askOutputOnce(do, &last, "GB7AAA", "serial") // the same again asks nothing
	askOutputOnce(do, &last, "GB7AAA", "emulator")
	if len(asked) != 2 {
		t.Fatalf("subscribed %d times, want 2: %+v", len(asked), asked)
	}
	if asked[0]["source"] != "serial" || asked[1]["source"] != "emulator" {
		t.Errorf("subscribed to %v", asked)
	}
}

// A popped-out window keeps the log it was opened for.
//
// The pane's own build() defaulted the source to serial, which ran after the
// window had set the one it was opened for - so every log window showed the
// serial log under its own name.
func TestAPoppedOutWindowKeepsTheLogItWasOpenedFor(t *testing.T) {
	p := &OutputWindowPanel{Node: "GB7AAA"}
	p.out.source, p.out.noPop = "emulator", true
	p.out.build()
	if p.out.source != "emulator" {
		t.Errorf("the window was opened on the emulator log and built as %q", p.out.source)
	}
	// And a pane nobody has pointed anywhere still starts on serial, which is
	// what the tab wants.
	o := &outputPane{}
	o.build()
	if o.source != "serial" {
		t.Errorf("a fresh pane starts on %q, want serial", o.source)
	}
}
