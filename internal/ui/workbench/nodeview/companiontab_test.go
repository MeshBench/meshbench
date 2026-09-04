// The companion client acts through verbs.
//
// Moved here with the node view: companionTab is this package's, and a test of
// it belongs beside it rather than in the workbench's control tests.
package nodeview

import (
	"strings"
	"testing"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
	"github.com/MeshBench/meshbench/internal/ui/uitest"
)

// The client acts through verbs, not through command-line strings.
//
// Every button used to format a meshcore-cli line, which is why the panes
// could only ever show a terminal: the answer came back as text meant for
// one. Sending, scope, adverts and refresh are verbs now, and the CLI is a
// mode beside them rather than the thing underneath them.
func TestTheCompanionClientActsThroughVerbs(t *testing.T) {
	var verbs []string
	var lines []string
	c := &companionTab{
		node:  "AngusOutlaw1",
		OnCLI: func(_, line string) { lines = append(lines, line) },
		OnDo:  func(verb string, _ any) { verbs = append(verbs, verb) },
	}
	// The flat layout: the real one hides most controls behind the modes.
	h := uitest.New(
		func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return c.AuditDraw(t, gtx, s)
		}, &state.Snapshot{})
	h.Frame()
	c.msg.Editor.SetText("hello fife")
	c.scope.Editor.SetText("#sco")
	c.cmd.Editor.SetText("infos")
	h.Frame()
	for y := float32(6); y < 340; y += 10 {
		h.PressAlong(y)
	}

	joined := strings.Join(verbs, " | ")
	for _, want := range []string{
		"companion.connect", "companion.send", "companion.scope",
		"companion.advert", "companion.refresh", "bench.serve", "bench.drop",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("no control reached %q; got: %s", want, joined)
		}
	}
	// And the command line still goes out as a command line.
	if !strings.Contains(strings.Join(lines, " | "), "infos") {
		t.Errorf("the CLI box did not send its line; got: %v", lines)
	}
}

// Serving hands the port to somebody else, so it has to let go of it first.
// Two holders of one claim is the thing the whole tab is arranged to prevent.
func TestServingDisconnectsFirst(t *testing.T) {
	var verbs []string
	c := &companionTab{
		node: "AngusOutlaw1",
		OnDo: func(verb string, _ any) { verbs = append(verbs, verb) },
	}
	c.build()
	snap := &state.Snapshot{Companions: []state.Companion{
		{Node: "AngusOutlaw1", Connected: true},
	}}
	h := uitest.New(
		func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return c.AuditDraw(t, gtx, s)
		}, snap)
	h.Frame()
	for y := float32(6); y < 340; y += 10 {
		h.PressAlong(y)
	}
	var disconnectAt, serveAt = -1, -1
	for i, v := range verbs {
		if v == "companion.disconnect" && disconnectAt < 0 {
			disconnectAt = i
		}
		if v == "bench.serve" && serveAt < 0 {
			serveAt = i
		}
	}
	if serveAt < 0 {
		t.Fatalf("Serve reached nothing: %v", verbs)
	}
	if disconnectAt < 0 || disconnectAt > serveAt {
		t.Errorf("served without releasing the claim first: %v", verbs)
	}
}
