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
	"strings"

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
	// A companion is what makes the decode tick worth offering, and the audit
	// needs it offered whatever node it happens to be pointed at - a control
	// that appears for one kind of node is one a sweep of another kind never
	// finds, and reports as unreachable.
	p.framed = p.auditing || isCompanion(s, p.Node)
	p.readTyping(gtx, s)
	return onSunk(t, gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: t.Sp.S, Right: t.Sp.S, Top: t.Sp.XS,
			Bottom: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.logHead(t, gtx, pane)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return p.logLines(t, gtx, s, pane)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.typeLine(t, gtx)
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
	// Only where there is something framed to decode. A repeater's console is
	// text already, and a tick offering to make text readable is a control
	// that can only puzzle whoever finds it.
	if p.framed {
		p.decode.Label = "decode"
		kids = append(kids,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.decode.Layout(t, gtx)
			}),
			layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout))
	}
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
func (p *Panel) logLines(t *theme.Theme, gtx layout.Context, s *state.Snapshot,
	pane *state.OutputPane) layout.Dimensions {

	// The wire by default, and the decoded exchange when it is asked for.
	//
	// A companion's serial carries the framed protocol, so a byte at a time it
	// is a wall of \x00\x05 with the answer buried in it - typing "ver" at one
	// shows the board name and the firmware version legible inside the escapes.
	// That is still what the board actually sent, and this window is about what
	// the board actually did, so it stays the default; the tick turns it into
	// the transcript console.cli has already decoded, which is the same one the
	// node window's Companion tab draws.
	if p.decode.Bool.Value && logSources[p.logSrc].key == "serial" {
		if lines, ok := companionTranscript(s, p.Node); ok {
			return p.drawLines(t, gtx, lines,
				"nothing typed at this companion yet - it answers meshcore-cli's "+
					"vocabulary, and ? lists it")
		}
	}
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
	return p.drawLines(t, gtx, pane.Lines, "")
}

// drawLines is the pane itself: a tail, or why there is none.
func (p *Panel) drawLines(t *theme.Theme, gtx layout.Context, lines []string,
	empty string) layout.Dimensions {

	if len(lines) == 0 && empty != "" {
		return comp.Text(t, t.Sz.Caption, t.P.Faint, empty)(gtx)
	}
	p.logList.Axis = layout.Vertical
	// Pinned to the newest line until somebody scrolls up, and pinned again
	// the moment they come back to the bottom.
	//
	// Gio's list does the whole of that from this one flag: with it set it
	// stays at the end once it has reached it, and it stops when the position
	// moves off the end - which is what scrolling up does - and resumes when
	// the position returns. A log strip that did not follow is one somebody
	// has to drag after every line, which for a board that is talking is every
	// few hundred milliseconds.
	p.logList.ScrollToEnd = true
	// Filled rather than sized to its longest line. A list takes the width its
	// content wants, and its scrollbar rides its right edge - so a log of short
	// lines put the bar somewhere in the middle of the strip, which reads as a
	// panel that has been cut off rather than as a list that fits.
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return comp.List(t, &p.logList, len(lines),
		func(gtx layout.Context, i int) layout.Dimensions {
			return comp.OneLine(t, t.Sz.Caption, t.P.Dim, lines[i], true)(gtx)
		})(gtx)
}

// companionTranscript is the decoded exchange for this node, where it is one.
//
// Keyed by node, because the transcript is one node's at a time: a board view
// on a different node than the one last typed at must not draw somebody else's
// conversation.
func companionTranscript(s *state.Snapshot, node string) ([]string, bool) {
	if s == nil || !isCompanion(s, node) {
		return nil, false
	}
	if s.ConsoleNode != node {
		return nil, true
	}
	return s.Console, true
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

// logHeights are how tall the strip may be pulled, in dp.
//
// The floor holds its two tabs and a line: a strip shrunk past that is one
// nobody can read or grab again. The ceiling leaves the table its own room -
// this is the log beside the board, and the Output tab next door is where the
// whole of it lives.
const (
	logMinDp     = 56
	logMaxDp     = 460
	logDefaultDp = 112
)

// logHeight is how tall the strip is drawn, in dp.
func (p *Panel) logHeight() int {
	if p.logH == 0 {
		p.logH = logDefaultDp
	}
	if p.logH < logMinDp {
		p.logH = logMinDp
	}
	if p.logH > logMaxDp {
		p.logH = logMaxDp
	}
	return p.logH
}

// dragLog is the rule above the strip, which is also how it is made taller.
//
// Dragged up, the log grows and the table gives way; there is no third thing
// to redistribute, which is what makes a single number enough here where the
// rail needed a scale.
func (p *Panel) dragLog(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	d := p.logSplit.Layout(t, gtx)
	if p.logSplit.Delta != 0 {
		perDp := float32(gtx.Dp(1))
		if perDp < 1 {
			perDp = 1
		}
		p.logH = p.logHeight() - int(p.logSplit.Delta/perDp)
	}
	return d
}

// typeLine is the console's own input: a line to the board, and the button for
// somebody who would rather press than press Enter.
//
// Always there, on either tab. The tabs choose what is being read and this
// chooses what is sent, which are different questions - and a box that came and
// went as somebody looked at the emulator log would be one they had to put back
// before they could type. It was hidden under the emulator tab for a few
// minutes, and the first thing that noticed was the control audit sweeping the
// panel: it pressed the tab, the box vanished, and the send button was never
// reachable again.
func (p *Panel) typeLine(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	p.typed.Hint = "type a command, then Enter"
	p.typed.Editor.SingleLine = true
	p.typed.Editor.Submit = true
	p.send.Kind, p.send.Label = comp.Quiet, "send"
	return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return p.typed.Layout(t, gtx)
				}),
				layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.send.Layout(t, gtx)
				}),
			)
		})
}

// readTyping sends what was typed, on Enter or on the button.
//
// Nothing is echoed here. console.type already writes the line into the
// console buffer this pane draws - buf.Echo, before the bytes go to the board -
// so a second copy from the interface would show every command twice.
//
// Routed by what the node is, because the two consoles take different things:
// a repeater reads typed text, and a companion speaks a framed protocol whose
// command line is meshcore-cli's vocabulary. Text typed at a companion through
// the repeater's verb is echoed locally and goes nowhere, which looks exactly
// like a command that ran and did nothing.
func (p *Panel) readTyping(gtx layout.Context, s *state.Snapshot) {
	submitted := false
	for {
		ev, ok := p.typed.Editor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			submitted = true
		}
	}
	if !submitted && !p.send.Click.Clicked(gtx) {
		return
	}
	line := strings.TrimSpace(p.typed.Editor.Text())
	if line == "" || p.OnDo == nil {
		return
	}
	verb := "console.type"
	if isCompanion(s, p.Node) {
		verb = "console.cli"
	}
	p.OnDo(verb, map[string]any{"node": p.Node, "command": line})
	p.typed.Editor.SetText("")
}

// isCompanion reports whether this node speaks the framed protocol rather than
// reading typed text.
func isCompanion(s *state.Snapshot, node string) bool {
	if s == nil {
		return false
	}
	for _, n := range s.Nodes {
		if n.Name == node {
			return strings.Contains(strings.ToLower(string(n.Kind)), "companion")
		}
	}
	return false
}
