package workbench

import (
	"slices"
	"testing"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// Every node that runs firmware grows an Output tab, whatever it runs and
// wherever it runs it.
//
// The question the tab answers - what did this thing print - is the same for a
// board under an emulator and a build on this machine. Only an observer is
// exempt, because an observer runs no firmware and has nothing to have said.
func TestEveryFirmwareNodeGetsTheOutputTab(t *testing.T) {
	for _, kind := range []string{"companion", "simple-repeater", "room-server"} {
		p := &nodeWindowPanel{}
		p.Kind = kind
		if !slices.Contains(p.visibleTabs(), tabOutput) {
			t.Errorf("a %s has no Output tab, so what it printed is unreachable", kind)
		}
	}
	p := &nodeWindowPanel{}
	p.Kind = "sdr-observer"
	if slices.Contains(p.visibleTabs(), tabOutput) {
		t.Error("an SDR observer offers an Output tab; it runs no firmware")
	}
}

// The pane shows this node's output and no other's.
//
// The snapshot carries one node's output at a time, the way it carries one
// node's console. Two windows open on two boards, and the one that asked last
// wins - so a pane that drew whatever the snapshot held would show one board's
// boot chain under the other board's name. That is worse than an empty pane:
// it is a wrong answer to the question the pane exists for.
func TestThePaneRefusesAnotherNodesOutput(t *testing.T) {
	o := &outputPane{}
	o.build()
	s := &state.Snapshot{
		Output:       []string{"ets Jul 29 2019", "boot ok"},
		OutputNode:   "GB7AAA",
		OutputSource: "serial",
		OutputTotal:  2,
	}
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
	p := &nodeWindowPanel{node: "GB7XYZ"}
	p.out.build()
	if !p.out.follow {
		t.Fatal("the pane does not follow by default; the newest line is the one being waited for")
	}
	h := newPanelHarness(func(_ *theme.Theme, gtx layout.Context, _ *state.Snapshot) layout.Dimensions {
		p.outputClicks(gtx)
		return layout.Dimensions{}
	}, nil)
	p.out.pauseBtn.Click.Click()
	h.frame()
	if p.out.follow {
		t.Error("pressing pause left the pane still chasing the end of the file")
	}
	if p.out.pauseBtn.Label != "follow" {
		t.Errorf("the button still says %q, so nothing says how to resume", p.out.pauseBtn.Label)
	}
	p.out.pauseBtn.Click.Click()
	h.frame()
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
	p := &nodeWindowPanel{node: "GB7XYZ"}
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

	h := newPanelHarness(func(_ *theme.Theme, gtx layout.Context, _ *state.Snapshot) layout.Dimensions {
		p.outputClicks(gtx)
		p.askOutput(p.out.source)
		return layout.Dimensions{}
	}, nil)
	p.out.srcBtns[1].Click.Click() // emulator
	h.frame()
	if got := asked[len(asked)-1]; got != "emulator" {
		t.Errorf("choosing the emulator asked for %q", got)
	}
	before := len(asked)
	p.out.srcBtns[1].Click.Click() // the same one again
	h.frame()
	if len(asked) == before {
		t.Error("pressing the source already showing asked for nothing, so it cannot be re-read")
	}
}
