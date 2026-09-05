// What the board and its emulator are saying, along the bottom.
//
// The reason this window is a window rather than a tab: the move it exists for
// is "the log said something, so what did the hardware do", and that needs the
// log and the table on screen at once. A strip that has to be switched to is a
// strip somebody reads afterwards, by which time they have lost the row they
// were looking at.
//
// Two voices, because they answer different questions. The console is what the
// firmware chose to print; the emulator is what QEMU or Renode said about
// running it. A board that says nothing on one and a great deal on the other is
// telling you which half of the problem you have - and the emulator's half is
// invisible from inside the guest, which is exactly when somebody needs it.
package boardview

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// logSource is one of the two voices, and the verb's own name for it.
type logSource struct{ key, label, what string }

// logSources are the two, in the order somebody reaches for them.
//
// The board's own first: it is what a person was reading when they opened this
// window. "Emulator" rather than "QEMU" or "Renode", because which one is
// running is a property of the board's MCU and not a choice anybody made - and
// the whole point of this window is that it reads the same either way.
var logSources = []logSource{
	{"serial", "Console", "what the board itself printed"},
	{"emulator", "Emulator", "what QEMU or Renode said about running it"},
}

// logStrip draws the tail of one source, with the other a click away.
func (p *Panel) logStrip(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	for i := range p.logTabs {
		if p.logTabs[i].Clicked(gtx) {
			p.logSrc = i
		}
	}
	// Asked for once per change rather than once a frame: the pane is fetched
	// by a verb, and a window drawing it every frame would ask for ever.
	askOutputOnce(p.OnDo, &p.logAsked, p.Node, logSources[p.logSrc].key)

	pane := paneFor(s, p.Node, logSources[p.logSrc].key)
	return onSunk(t, gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: t.Sp.S, Right: t.Sp.S, Top: t.Sp.XS,
			Bottom: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.logHead(t, gtx, pane)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return p.logLines(t, gtx, pane)
				}),
			)
		})
	})
}

// logHead is the two tabs and what the pane can say about itself.
func (p *Panel) logHead(t *theme.Theme, gtx layout.Context,
	pane *state.OutputPane) layout.Dimensions {

	var kids []layout.FlexChild
	for i := range logSources {
		src := logSources[i]
		ink := t.P.Faint
		if i == p.logSrc {
			ink = t.P.Ink
		}
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.logTabs[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: t.Sp.M}.Layout(gtx,
					comp.Text(t, t.Sz.Caption, ink, src.label))
			})
		}))
	}
	kids = append(kids, layout.Flexed(1, spacer))
	// What the file holds against what is shown, so a tail does not read as
	// the whole of it.
	if pane != nil && pane.Total > len(pane.Lines) {
		kids = append(kids, layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Faint,
			fmt.Sprintf("last %d of %d", len(pane.Lines), pane.Total))))
	}
	return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
		})
}

// logLines is the tail itself, or why there is none.
func (p *Panel) logLines(t *theme.Theme, gtx layout.Context,
	pane *state.OutputPane) layout.Dimensions {

	if pane == nil {
		return comp.Text(t, t.Sz.Caption, t.P.Faint,
			"nothing has been read from this node yet")(gtx)
	}
	if len(pane.Lines) == 0 {
		// A source that is empty for a reason says the reason. A board whose
		// console is on USB has nothing on UART0 after the bootloader, and a
		// blank pane with no explanation reads as a broken one.
		what := pane.Note
		if what == "" {
			what = "nothing on this " + logSources[p.logSrc].label
		}
		return comp.Text(t, t.Sz.Caption, t.P.Faint, what)(gtx)
	}
	p.logList.Axis = layout.Vertical
	return comp.List(t, &p.logList, len(pane.Lines),
		func(gtx layout.Context, i int) layout.Dimensions {
			return comp.OneLine(t, t.Sz.Caption, t.P.Dim, pane.Lines[i], true)(gtx)
		})(gtx)
}

// paneFor is this node's pane for one source, or nil before one has arrived.
func paneFor(s *state.Snapshot, node, source string) *state.OutputPane {
	if s == nil {
		return nil
	}
	for i := range s.Outputs {
		if s.Outputs[i].Node == node && s.Outputs[i].Source == source {
			return &s.Outputs[i]
		}
	}
	return nil
}

// askOutputOnce points the session at one node and source, once per change.
//
// The same shape the node window uses, and deliberately not the same function:
// that one is unexported in nodeview and this package cannot reach it. Two
// callers of a three-line guard is not the duplication worth solving; two
// copies of a board's part renderers was.
func askOutputOnce(do func(verb string, params any), asked *string, node, source string) {
	if do == nil {
		return
	}
	want := node + "/" + source
	if want == *asked {
		return
	}
	*asked = want
	do("node.output", map[string]any{"node": node, "source": source})
}

var _ = widget.Clickable{}
